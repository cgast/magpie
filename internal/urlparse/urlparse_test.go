package urlparse

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		hint    Kind
		want    Kind
		id      string
		cloud   string
		tql     string
	}{
		{"issue browse", "https://example.atlassian.net/browse/ABC-123", Board, Issue, "ABC-123", "", ""},
		{"issue selected", "https://example.atlassian.net/jira/software/projects/ABC/boards/5?selectedIssue=ABC-9", Board, Issue, "ABC-9", "", ""},
		{"board url", "https://example.atlassian.net/jira/software/projects/ABC/boards/42", Board, Board, "42", "", ""},
		{"board classic", "https://example.atlassian.net/secure/RapidBoard.jspa?rapidView=7", Board, Board, "7", "", ""},
		{"conf page", "https://example.atlassian.net/wiki/spaces/ENG/pages/98765/Runbook", Page, Page, "98765", "", ""},
		{"goal detail", "https://home.atlassian.com/o/abc123/goal/GOAL-4", Goal, Goal, "GOAL-4", "", ""},
		{"goal list", "https://home.atlassian.com/o/abc123/goals?viewTab=all&cloudId=00000000-0000-0000-0000-000000000000&tql=(archived+%3D+false)", Goal, GoalList, "", "00000000-0000-0000-0000-000000000000", "(archived = false)"},
		{"bare issue key", "ABC-77", Board, Issue, "ABC-77", "", ""},
		{"bare number as board", "42", Board, Board, "42", "", ""},
		{"bare number as page", "98765", Page, Page, "98765", "", ""},
		{"bare goal key", "GOAL-4", Goal, Goal, "GOAL-4", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in, c.hint)
			if got.Kind != c.want {
				t.Errorf("Kind = %v, want %v", got.Kind, c.want)
			}
			if got.ID != c.id {
				t.Errorf("ID = %q, want %q", got.ID, c.id)
			}
			if c.cloud != "" && got.CloudID != c.cloud {
				t.Errorf("CloudID = %q, want %q", got.CloudID, c.cloud)
			}
			if c.tql != "" && got.TQL != c.tql {
				t.Errorf("TQL = %q, want %q", got.TQL, c.tql)
			}
		})
	}
}
