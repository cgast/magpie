// Package jira implements the read-only Jira surface: a single issue, a board
// (columns + issues), and JQL search. It returns normalized structs so the JSON
// contract stays stable regardless of Jira API shape changes.
package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/cgast/magpie/internal/adf"
	"github.com/cgast/magpie/internal/httpclient"
)

// Service holds the dependencies for Jira calls.
type Service struct {
	Client *httpclient.Client
	Site   string // host, e.g. "example.atlassian.net"
}

func (s Service) base() string { return "https://" + s.Site }

// ---------- normalized entities ----------

type Comment struct {
	Author  string `json:"author"`
	Created string `json:"created"`
	Body    string `json:"body"`
}

type Issue struct {
	Key         string    `json:"key"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority,omitempty"`
	Assignee    string    `json:"assignee,omitempty"`
	Reporter    string    `json:"reporter,omitempty"`
	Labels      []string  `json:"labels,omitempty"`
	Created     string    `json:"created,omitempty"`
	Updated     string    `json:"updated,omitempty"`
	Description string    `json:"description,omitempty"`
	Comments    []Comment `json:"comments,omitempty"`
}

func (i Issue) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n\n", i.Key, i.Summary)
	fmt.Fprintf(&b, "- **Type:** %s\n- **Status:** %s\n", i.Type, i.Status)
	if i.Priority != "" {
		fmt.Fprintf(&b, "- **Priority:** %s\n", i.Priority)
	}
	if i.Assignee != "" {
		fmt.Fprintf(&b, "- **Assignee:** %s\n", i.Assignee)
	}
	if i.Reporter != "" {
		fmt.Fprintf(&b, "- **Reporter:** %s\n", i.Reporter)
	}
	if len(i.Labels) > 0 {
		fmt.Fprintf(&b, "- **Labels:** %s\n", strings.Join(i.Labels, ", "))
	}
	fmt.Fprintf(&b, "- **URL:** %s\n", i.URL)
	if i.Description != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", i.Description)
	}
	if len(i.Comments) > 0 {
		b.WriteString("\n## Comments\n\n")
		for _, c := range i.Comments {
			fmt.Fprintf(&b, "**%s** (%s):\n\n%s\n\n", c.Author, c.Created, c.Body)
		}
	}
	return b.String()
}

type IssueBrief struct {
	Key      string `json:"key"`
	URL      string `json:"url"`
	Summary  string `json:"summary"`
	Type     string `json:"type,omitempty"`
	Status   string `json:"status"`
	Assignee string `json:"assignee,omitempty"`
}

type SearchResult struct {
	JQL    string       `json:"jql"`
	Count  int          `json:"count"`
	Issues []IssueBrief `json:"issues"`
}

func (r SearchResult) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# JQL: `%s`\n\n%d issue(s)\n\n", r.JQL, r.Count)
	for _, i := range r.Issues {
		assignee := i.Assignee
		if assignee == "" {
			assignee = "Unassigned"
		}
		fmt.Fprintf(&b, "- **%s** [%s] %s _(%s)_\n", i.Key, i.Status, i.Summary, assignee)
	}
	return b.String()
}

type Column struct {
	Name   string       `json:"name"`
	Issues []IssueBrief `json:"issues"`
}

type Board struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	URL     string   `json:"url"`
	Columns []Column `json:"columns"`
}

func (bd Board) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Board: %s (%s)\n\n%s\n\n", bd.Name, bd.Type, bd.URL)
	for _, col := range bd.Columns {
		fmt.Fprintf(&b, "## %s (%d)\n\n", col.Name, len(col.Issues))
		for _, i := range col.Issues {
			fmt.Fprintf(&b, "- **%s** %s\n", i.Key, i.Summary)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ---------- raw API shapes ----------

type apiUser struct {
	DisplayName string `json:"displayName"`
}

type apiIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary   string          `json:"summary"`
		IssueType struct{ Name string } `json:"issuetype"`
		Status    struct{ Name string } `json:"status"`
		Priority  *struct{ Name string } `json:"priority"`
		Assignee  *apiUser        `json:"assignee"`
		Reporter  *apiUser        `json:"reporter"`
		Labels    []string        `json:"labels"`
		Created   string          `json:"created"`
		Updated   string          `json:"updated"`
		Desc      json.RawMessage `json:"description"`
		Comment   *struct {
			Comments []struct {
				Author  apiUser         `json:"author"`
				Created string          `json:"created"`
				Body    json.RawMessage `json:"body"`
			} `json:"comments"`
		} `json:"comment"`
	} `json:"fields"`
}

func (s Service) issueURL(key string) string { return s.base() + "/browse/" + key }

// ReadIssue fetches one issue and its comments.
func (s Service) ReadIssue(ctx context.Context, key string) (Issue, error) {
	fields := "summary,issuetype,status,priority,assignee,reporter,labels,created,updated,description,comment"
	u := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=%s", s.base(), url.PathEscape(key), fields)
	body, err := s.Client.GetJSON(ctx, u)
	if err != nil {
		return Issue{}, err
	}
	var raw apiIssue
	if err := json.Unmarshal(body, &raw); err != nil {
		return Issue{}, err
	}
	return s.toIssue(raw), nil
}

func (s Service) toIssue(raw apiIssue) Issue {
	i := Issue{
		Key:         raw.Key,
		URL:         s.issueURL(raw.Key),
		Summary:     raw.Fields.Summary,
		Type:        raw.Fields.IssueType.Name,
		Status:      raw.Fields.Status.Name,
		Labels:      raw.Fields.Labels,
		Created:     raw.Fields.Created,
		Updated:     raw.Fields.Updated,
		Description: adf.Render(raw.Fields.Desc),
	}
	if raw.Fields.Priority != nil {
		i.Priority = raw.Fields.Priority.Name
	}
	if raw.Fields.Assignee != nil {
		i.Assignee = raw.Fields.Assignee.DisplayName
	}
	if raw.Fields.Reporter != nil {
		i.Reporter = raw.Fields.Reporter.DisplayName
	}
	if raw.Fields.Comment != nil {
		for _, c := range raw.Fields.Comment.Comments {
			i.Comments = append(i.Comments, Comment{
				Author:  c.Author.DisplayName,
				Created: c.Created,
				Body:    adf.Render(c.Body),
			})
		}
	}
	return i
}

// Search runs a JQL query using the current (non-deprecated) search endpoint.
func (s Service) Search(ctx context.Context, jql string, max int) (SearchResult, error) {
	payload := map[string]any{
		"jql":        jql,
		"maxResults": max,
		"fields":     []string{"summary", "status", "assignee", "issuetype"},
	}
	body, err := s.Client.PostJSON(ctx, s.base()+"/rest/api/3/search/jql", payload)
	if err != nil {
		return SearchResult{}, err
	}
	var raw struct {
		Issues []apiIssue `json:"issues"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return SearchResult{}, err
	}
	res := SearchResult{JQL: jql, Count: len(raw.Issues)}
	for _, ri := range raw.Issues {
		res.Issues = append(res.Issues, s.brief(ri))
	}
	return res, nil
}

func (s Service) brief(ri apiIssue) IssueBrief {
	b := IssueBrief{
		Key:     ri.Key,
		URL:     s.issueURL(ri.Key),
		Summary: ri.Fields.Summary,
		Type:    ri.Fields.IssueType.Name,
		Status:  ri.Fields.Status.Name,
	}
	if ri.Fields.Assignee != nil {
		b.Assignee = ri.Fields.Assignee.DisplayName
	}
	return b
}

// ReadBoard fetches a board with its columns and current issues.
func (s Service) ReadBoard(ctx context.Context, boardID string, max int) (Board, error) {
	// Board metadata.
	metaBody, err := s.Client.GetJSON(ctx, fmt.Sprintf("%s/rest/agile/1.0/board/%s", s.base(), boardID))
	if err != nil {
		return Board{}, err
	}
	var meta struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return Board{}, err
	}

	// Column configuration: maps status ids to columns.
	cfgBody, err := s.Client.GetJSON(ctx, fmt.Sprintf("%s/rest/agile/1.0/board/%s/configuration", s.base(), boardID))
	if err != nil {
		return Board{}, err
	}
	var cfg struct {
		ColumnConfig struct {
			Columns []struct {
				Name     string `json:"name"`
				Statuses []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			} `json:"columns"`
		} `json:"columnConfig"`
	}
	if err := json.Unmarshal(cfgBody, &cfg); err != nil {
		return Board{}, err
	}

	// Issues on the board.
	issuesBody, err := s.Client.GetJSON(ctx, fmt.Sprintf(
		"%s/rest/agile/1.0/board/%s/issue?maxResults=%d&fields=summary,status,assignee,issuetype",
		s.base(), boardID, max))
	if err != nil {
		return Board{}, err
	}
	var issuesResp struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary   string                `json:"summary"`
				IssueType struct{ Name string } `json:"issuetype"`
				Status    struct {
					Name string `json:"name"`
					ID   string `json:"id"`
				} `json:"status"`
				Assignee *apiUser `json:"assignee"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(issuesBody, &issuesResp); err != nil {
		return Board{}, err
	}

	// statusID -> column index
	colOf := map[string]int{}
	board := Board{ID: meta.ID, Name: meta.Name, Type: meta.Type,
		URL: fmt.Sprintf("%s/jira/software/boards/%s", s.base(), boardID)}
	for idx, c := range cfg.ColumnConfig.Columns {
		board.Columns = append(board.Columns, Column{Name: c.Name})
		for _, st := range c.Statuses {
			colOf[st.ID] = idx
		}
	}
	// Bucket for issues whose status isn't mapped to a column.
	other := Column{Name: "(unmapped)"}
	for _, wi := range issuesResp.Issues {
		br := IssueBrief{
			Key:     wi.Key,
			URL:     s.issueURL(wi.Key),
			Summary: wi.Fields.Summary,
			Type:    wi.Fields.IssueType.Name,
			Status:  wi.Fields.Status.Name,
		}
		if wi.Fields.Assignee != nil {
			br.Assignee = wi.Fields.Assignee.DisplayName
		}
		if idx, ok := colOf[wi.Fields.Status.ID]; ok {
			board.Columns[idx].Issues = append(board.Columns[idx].Issues, br)
		} else {
			other.Issues = append(other.Issues, br)
		}
	}
	if len(other.Issues) > 0 {
		board.Columns = append(board.Columns, other)
	}
	return board, nil
}
