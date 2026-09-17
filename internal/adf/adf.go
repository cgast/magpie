// Package adf converts Atlassian Document Format (the JSON rich-text used by
// Jira issue bodies, Confluence v2 page bodies, and Goal summaries) into
// Markdown. It handles the common node set; unknown nodes degrade to their
// text content rather than failing.
package adf

import (
	"encoding/json"
	"fmt"
	"strings"
)

type node struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Content []node          `json:"content"`
	Marks   []mark          `json:"marks"`
	Attrs   json.RawMessage `json:"attrs"`
}

type mark struct {
	Type  string          `json:"type"`
	Attrs json.RawMessage `json:"attrs"`
}

// Render parses an ADF document (given as raw JSON) and returns Markdown.
// An empty or unparseable document yields an empty string.
func Render(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var doc node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	var b strings.Builder
	renderNodes(&b, doc.Content, "")
	return strings.TrimRight(b.String(), "\n")
}

// RenderString is a convenience for ADF stored as a JSON string (Confluence v2
// returns body.atlas_doc_format.value as a string).
func RenderString(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return Render(json.RawMessage(s))
}

func renderNodes(b *strings.Builder, nodes []node, listPrefix string) {
	for _, n := range nodes {
		renderNode(b, n, listPrefix)
	}
}

func renderNode(b *strings.Builder, n node, listPrefix string) {
	switch n.Type {
	case "paragraph":
		b.WriteString(inline(n.Content))
		b.WriteString("\n\n")
	case "heading":
		level := attrInt(n.Attrs, "level", 1)
		b.WriteString(strings.Repeat("#", level) + " " + inline(n.Content) + "\n\n")
	case "bulletList":
		renderList(b, n.Content, "- ")
		b.WriteString("\n")
	case "orderedList":
		renderOrderedList(b, n.Content)
		b.WriteString("\n")
	case "listItem":
		b.WriteString(listPrefix + inlineBlocks(n.Content) + "\n")
	case "codeBlock":
		lang := attrString(n.Attrs, "language")
		b.WriteString("```" + lang + "\n" + inline(n.Content) + "\n```\n\n")
	case "blockquote":
		var inner strings.Builder
		renderNodes(&inner, n.Content, "")
		for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			b.WriteString("> " + line + "\n")
		}
		b.WriteString("\n")
	case "rule":
		b.WriteString("---\n\n")
	case "table":
		renderTable(b, n.Content)
	case "mediaSingle", "mediaGroup":
		b.WriteString("[media]\n\n")
	default:
		// Unknown block: render any inline content we can find.
		if txt := inline(n.Content); txt != "" {
			b.WriteString(txt + "\n\n")
		} else if n.Text != "" {
			b.WriteString(n.Text + "\n\n")
		}
	}
}

func renderList(b *strings.Builder, items []node, bullet string) {
	for _, item := range items {
		b.WriteString(bullet + inlineBlocks(item.Content) + "\n")
	}
}

func renderOrderedList(b *strings.Builder, items []node) {
	for i, item := range items {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, inlineBlocks(item.Content)))
	}
}

// inlineBlocks flattens a listItem's child paragraphs into a single line.
func inlineBlocks(nodes []node) string {
	var parts []string
	for _, n := range nodes {
		if s := strings.TrimSpace(inline(n.Content)); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func renderTable(b *strings.Builder, rows []node) {
	for r, row := range rows {
		var cells []string
		for _, cell := range row.Content {
			cells = append(cells, strings.TrimSpace(inlineBlocks(cell.Content)))
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if r == 0 {
			sep := make([]string, len(cells))
			for i := range sep {
				sep[i] = "---"
			}
			b.WriteString("| " + strings.Join(sep, " | ") + " |\n")
		}
	}
	b.WriteString("\n")
}

// inline renders inline (text-level) nodes with their marks applied.
func inline(nodes []node) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case "text":
			b.WriteString(applyMarks(n.Text, n.Marks))
		case "hardBreak":
			b.WriteString("\n")
		case "mention":
			b.WriteString("@" + attrString(n.Attrs, "text"))
		case "emoji":
			b.WriteString(attrString(n.Attrs, "shortName"))
		case "inlineCard":
			b.WriteString(attrString(n.Attrs, "url"))
		default:
			if len(n.Content) > 0 {
				b.WriteString(inline(n.Content))
			} else if n.Text != "" {
				b.WriteString(n.Text)
			}
		}
	}
	return b.String()
}

func applyMarks(text string, marks []mark) string {
	for _, m := range marks {
		switch m.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "strike":
			text = "~~" + text + "~~"
		case "link":
			if href := attrString(m.Attrs, "href"); href != "" {
				text = "[" + text + "](" + href + ")"
			}
		}
	}
	return text
}

// --- small attribute helpers ---

func attrString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	m := map[string]any{}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func attrInt(raw json.RawMessage, key string, def int) int {
	if len(raw) == 0 {
		return def
	}
	m := map[string]any{}
	if json.Unmarshal(raw, &m) != nil {
		return def
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return def
}
