// Package httpclient centralizes every outbound call: basic auth, timeouts,
// rate-limit backoff, and error surfacing. Domain packages never build requests
// themselves, so auth and retry behavior live in exactly one place.
package httpclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cgast/magpie/internal/config"
)

// Client wraps http.Client with Atlassian basic auth.
type Client struct {
	http  *http.Client
	authz string
}

// New builds a Client from credentials. The Authorization header is precomputed
// once (email:token, base64) and reused for Jira REST, Confluence REST, and the
// Goals GraphQL gateway alike.
func New(c config.Config) *Client {
	raw := c.Email + ":" + c.Token
	return &Client{
		http:  &http.Client{Timeout: 30 * time.Second},
		authz: "Basic " + base64.StdEncoding.EncodeToString([]byte(raw)),
	}
}

const maxRetries = 3

// do performs a request with auth, retrying on HTTP 429 up to maxRetries times,
// honoring Retry-After when present.
func (c *Client) do(ctx context.Context, method, url string, body io.Reader, contentType string) ([]byte, error) {
	var raw []byte
	if body != nil {
		var err error
		raw, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}

	for attempt := 0; ; attempt++ {
		var rdr io.Reader
		if raw != nil {
			rdr = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rdr)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", c.authz)
		req.Header.Set("Accept", "application/json")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			wait := backoff(resp.Header.Get("Retry-After"), attempt)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, httpError(method, url, resp.StatusCode, data)
		}
		return data, nil
	}
}

// GetJSON issues a GET and returns the raw body.
func (c *Client) GetJSON(ctx context.Context, url string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, url, nil, "")
}

// PostJSON issues a POST with a JSON body.
func (c *Client) PostJSON(ctx context.Context, url string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPost, url, bytes.NewReader(b), "application/json")
}

// GraphQL posts a query to a GraphQL endpoint and returns the "data" object.
// GraphQL returns HTTP 200 even on query errors, so those are surfaced here.
func (c *Client) GraphQL(ctx context.Context, endpoint, query string, variables map[string]any) (json.RawMessage, error) {
	payload := map[string]any{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	body, err := c.PostJSON(ctx, endpoint, payload)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("graphql: bad response: %w", err)
	}
	if len(env.Errors) > 0 {
		return env.Data, fmt.Errorf("graphql error: %s", env.Errors[0].Message)
	}
	return env.Data, nil
}

func backoff(retryAfter string, attempt int) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(1<<attempt) * time.Second // 1s, 2s, 4s
}

func httpError(method, url string, code int, body []byte) error {
	msg := extractAPIMessage(body)
	if msg != "" {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, url, code, msg)
	}
	return fmt.Errorf("%s %s: HTTP %d", method, url, code)
}

// extractAPIMessage pulls a human-readable message out of the various error
// shapes Jira and Confluence return, so agents get a useful line instead of raw JSON.
func extractAPIMessage(body []byte) string {
	var j struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
		Message       string            `json:"message"`
		Title         string            `json:"title"`
	}
	if json.Unmarshal(body, &j) != nil {
		return ""
	}
	if len(j.ErrorMessages) > 0 {
		return j.ErrorMessages[0]
	}
	if j.Message != "" {
		return j.Message
	}
	if j.Title != "" {
		return j.Title
	}
	for k, v := range j.Errors {
		return k + ": " + v
	}
	return ""
}
