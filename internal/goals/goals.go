// Package goals implements the read-only Atlassian Home (Goals) surface.
//
// Goals has no REST API and no CLI — only a GraphQL gateway — and its schema is
// the least stable of the three products. The two queries below are kept as
// editable constants: if a field name differs on your tenant, verify it in the
// GraphiQL explorer (developer.atlassian.com Goals docs) and adjust here. The
// full GraphQL "data" object is always preserved in Raw so nothing is lost even
// if the normalized fields need tweaking.
package goals

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cgast/magpie/internal/httpclient"
)

type Service struct {
	Client  *httpclient.Client
	Site    string // tenant gateway host, e.g. "example.atlassian.net"
	CloudID string // tenant cloud id; auto-resolved if empty
}

func (s Service) endpoint() string { return "https://" + s.Site + "/gateway/api/graphql" }

func (s Service) ari(key string) string {
	return "ari:cloud:townsquare:" + s.CloudID + ":goal/" + key
}

// EnsureCloudID resolves the tenant cloud id from the site if not already set,
// using the unauthenticated tenant_info edge endpoint.
func (s *Service) EnsureCloudID(ctx context.Context) error {
	if s.CloudID != "" {
		return nil
	}
	body, err := s.Client.GetJSON(ctx, "https://"+s.Site+"/_edge/tenant_info")
	if err != nil {
		return fmt.Errorf("could not resolve cloudId from %s: %w (set MAGPIE_CLOUD_ID)", s.Site, err)
	}
	var t struct {
		CloudID string `json:"cloudId"`
	}
	if err := json.Unmarshal(body, &t); err != nil || t.CloudID == "" {
		return fmt.Errorf("could not parse cloudId from tenant_info (set MAGPIE_CLOUD_ID)")
	}
	s.CloudID = t.CloudID
	return nil
}

// ---------- editable GraphQL (verify in GraphiQL if a field differs) ----------

const listQuery = `query MagpieGoalList($cloudId: ID!, $q: String, $first: Int!) {
  goalTql(cloudId: $cloudId, q: $q, first: $first) {
    count
    edges { node { id key name state { label } dueDate progress { percentage } } }
  }
}`

const readQuery = `query MagpieGoal($id: ID!) {
  goal(id: $id) {
    id key name
    state { label }
    dueDate
    progress { percentage }
  }
}`

// ---------- normalized entities ----------

type Goal struct {
	ID       string          `json:"id,omitempty"`
	Key      string          `json:"key,omitempty"`
	Name     string          `json:"name,omitempty"`
	State    string          `json:"state,omitempty"`
	DueDate  string          `json:"dueDate,omitempty"`
	Progress *float64        `json:"progressPercent,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

func (g Goal) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Goal: %s\n\n", firstNonEmpty(g.Name, g.Key, g.ID))
	if g.Key != "" {
		fmt.Fprintf(&b, "- **Key:** %s\n", g.Key)
	}
	if g.State != "" {
		fmt.Fprintf(&b, "- **State:** %s\n", g.State)
	}
	if g.DueDate != "" {
		fmt.Fprintf(&b, "- **Due:** %s\n", g.DueDate)
	}
	if g.Progress != nil {
		fmt.Fprintf(&b, "- **Progress:** %.0f%%\n", *g.Progress)
	}
	return b.String()
}

type GoalsResult struct {
	TQL     string `json:"tql"`
	CloudID string `json:"cloudId"`
	Count   int    `json:"count"`
	Goals   []Goal `json:"goals"`
}

func (r GoalsResult) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Goals (TQL: `%s`)\n\n%d goal(s)\n\n", r.TQL, r.Count)
	for _, g := range r.Goals {
		state := g.State
		if state != "" {
			state = " [" + state + "]"
		}
		fmt.Fprintf(&b, "- **%s**%s %s\n", firstNonEmpty(g.Key, g.ID), state, g.Name)
	}
	return b.String()
}

// ---------- reads ----------

// nodeShape matches the fields our queries request; extra tenant fields are
// ignored by the normalizer but preserved in Raw.
type nodeShape struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	State   struct {
		Label string `json:"label"`
	} `json:"state"`
	DueDate  string `json:"dueDate"`
	Progress *struct {
		Percentage float64 `json:"percentage"`
	} `json:"progress"`
}

func toGoal(raw json.RawMessage) Goal {
	var n nodeShape
	_ = json.Unmarshal(raw, &n)
	g := Goal{ID: n.ID, Key: n.Key, Name: n.Name, State: n.State.Label, DueDate: n.DueDate, Raw: raw}
	if n.Progress != nil {
		p := n.Progress.Percentage
		g.Progress = &p
	}
	return g
}

// Read fetches a single goal by its key (or full ARI).
func (s *Service) Read(ctx context.Context, keyOrARI string) (Goal, error) {
	if err := s.EnsureCloudID(ctx); err != nil {
		return Goal{}, err
	}
	id := keyOrARI
	if !strings.HasPrefix(id, "ari:") {
		id = s.ari(keyOrARI)
	}
	data, err := s.Client.GraphQL(ctx, s.endpoint(), readQuery, map[string]any{"id": id})
	if err != nil {
		return Goal{}, fmt.Errorf("%w\n(hint: verify the Goals query fields in GraphiQL — see README)", err)
	}
	var wrap struct {
		Goal json.RawMessage `json:"goal"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return Goal{}, err
	}
	return toGoal(wrap.Goal), nil
}

// List fetches goals matching a TQL filter (default: non-archived).
func (s *Service) List(ctx context.Context, tql string, first int) (GoalsResult, error) {
	if err := s.EnsureCloudID(ctx); err != nil {
		return GoalsResult{}, err
	}
	if tql == "" {
		tql = "(archived = false)"
	}
	vars := map[string]any{"cloudId": s.CloudID, "q": tql, "first": first}
	data, err := s.Client.GraphQL(ctx, s.endpoint(), listQuery, vars)
	if err != nil {
		return GoalsResult{}, fmt.Errorf("%w\n(hint: verify the Goals query fields in GraphiQL — see README)", err)
	}
	var wrap struct {
		GoalTql struct {
			Count int `json:"count"`
			Edges []struct {
				Node json.RawMessage `json:"node"`
			} `json:"edges"`
		} `json:"goalTql"`
	}
	if err := json.Unmarshal(data, &wrap); err != nil {
		return GoalsResult{}, err
	}
	res := GoalsResult{TQL: tql, CloudID: s.CloudID, Count: wrap.GoalTql.Count}
	for _, e := range wrap.GoalTql.Edges {
		res.Goals = append(res.Goals, toGoal(e.Node))
	}
	if res.Count == 0 {
		res.Count = len(res.Goals)
	}
	return res, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
