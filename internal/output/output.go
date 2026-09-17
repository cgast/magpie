// Package output is the one place that decides how results reach stdout.
// Default is pretty JSON (a stable contract for a coding agent); --markdown
// switches to the human-readable rendering each entity provides.
package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// Renderable is implemented by every entity the tool can print. The JSON form
// comes from struct tags; Markdown is produced by the type itself.
type Renderable interface {
	Markdown() string
}

// Emit writes v to w as Markdown when markdown is true, otherwise as indented JSON.
func Emit(w io.Writer, v Renderable, markdown bool) error {
	if markdown {
		_, err := fmt.Fprintln(w, v.Markdown())
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
