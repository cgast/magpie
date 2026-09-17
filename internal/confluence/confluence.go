// Package confluence implements the read-only Confluence surface: a page (with
// an optional depth of child pages) and CQL search. Page bodies are fetched as
// ADF and rendered to Markdown via the shared adf package.
package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/cgast/magpie/internal/adf"
	"github.com/cgast/magpie/internal/httpclient"
)

type Service struct {
	Client *httpclient.Client
	Site   string
}

func (s Service) base() string { return "https://" + s.Site }

// ---------- normalized entities ----------

type Page struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	SpaceID  string `json:"spaceId,omitempty"`
	Version  int    `json:"version,omitempty"`
	URL      string `json:"url"`
	Body     string `json:"body,omitempty"`
	Children []Page `json:"children,omitempty"`
}

func (p Page) Markdown() string {
	var b strings.Builder
	p.markdownAt(&b, 1)
	return b.String()
}

func (p Page) markdownAt(b *strings.Builder, level int) {
	if level > 6 {
		level = 6
	}
	fmt.Fprintf(b, "%s %s\n\n%s\n\n", strings.Repeat("#", level), p.Title, p.URL)
	if p.Body != "" {
		b.WriteString(p.Body + "\n\n")
	}
	for _, c := range p.Children {
		c.markdownAt(b, level+1)
	}
}

type PageBrief struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

type CQLResult struct {
	CQL     string      `json:"cql"`
	Count   int         `json:"count"`
	Results []PageBrief `json:"results"`
}

func (r CQLResult) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# CQL: `%s`\n\n%d result(s)\n\n", r.CQL, r.Count)
	for _, p := range r.Results {
		fmt.Fprintf(&b, "- **%s** _(%s)_ — %s\n", p.Title, p.Type, p.URL)
	}
	return b.String()
}

// ---------- reads ----------

func (s Service) webURL(webui string) string {
	if webui == "" {
		return s.base() + "/wiki"
	}
	if strings.HasPrefix(webui, "/wiki") {
		return s.base() + webui
	}
	return s.base() + "/wiki" + webui
}

// ReadPage fetches a page and, when depth > 0, that many levels of child pages.
func (s Service) ReadPage(ctx context.Context, id string, depth int) (Page, error) {
	return s.fetchPage(ctx, id, depth)
}

func (s Service) fetchPage(ctx context.Context, id string, depth int) (Page, error) {
	u := fmt.Sprintf("%s/wiki/api/v2/pages/%s?body-format=atlas_doc_format",
		s.base(), url.PathEscape(id))
	body, err := s.Client.GetJSON(ctx, u)
	if err != nil {
		return Page{}, err
	}
	var raw struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		SpaceID string `json:"spaceId"`
		Version struct {
			Number int `json:"number"`
		} `json:"version"`
		Body struct {
			ADF struct {
				Value string `json:"value"`
			} `json:"atlas_doc_format"`
		} `json:"body"`
		Links struct {
			WebUI string `json:"webui"`
		} `json:"_links"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Page{}, err
	}
	p := Page{
		ID:      raw.ID,
		Title:   raw.Title,
		SpaceID: raw.SpaceID,
		Version: raw.Version.Number,
		URL:     s.webURL(raw.Links.WebUI),
		Body:    adf.RenderString(raw.Body.ADF.Value),
	}
	if depth > 0 {
		children, err := s.childIDs(ctx, id)
		if err != nil {
			return p, err
		}
		for _, cid := range children {
			child, err := s.fetchPage(ctx, cid, depth-1)
			if err != nil {
				return p, err
			}
			p.Children = append(p.Children, child)
		}
	}
	return p, nil
}

func (s Service) childIDs(ctx context.Context, id string) ([]string, error) {
	u := fmt.Sprintf("%s/wiki/api/v2/pages/%s/children?limit=100", s.base(), url.PathEscape(id))
	body, err := s.Client.GetJSON(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(raw.Results))
	for _, r := range raw.Results {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

// Search runs a CQL query.
func (s Service) Search(ctx context.Context, cql string, max int) (CQLResult, error) {
	u := fmt.Sprintf("%s/wiki/rest/api/search?cql=%s&limit=%d",
		s.base(), url.QueryEscape(cql), max)
	body, err := s.Client.GetJSON(ctx, u)
	if err != nil {
		return CQLResult{}, err
	}
	var raw struct {
		Results []struct {
			Content struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				Type  string `json:"type"`
				Links struct {
					WebUI string `json:"webui"`
				} `json:"_links"`
			} `json:"content"`
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return CQLResult{}, err
	}
	res := CQLResult{CQL: cql, Count: len(raw.Results)}
	for _, r := range raw.Results {
		title := r.Content.Title
		if title == "" {
			title = r.Title
		}
		link := r.Content.Links.WebUI
		if link == "" {
			link = r.URL
		}
		res.Results = append(res.Results, PageBrief{
			ID:    r.Content.ID,
			Title: title,
			Type:  r.Content.Type,
			URL:   s.webURL(link),
		})
	}
	return res, nil
}
