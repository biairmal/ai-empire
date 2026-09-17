package docs

import (
	"strings"
	"time"
)

// Example turns a template into a complete document with placeholder-free
// example content: every {{ … }} becomes "Example", guidance comments are
// removed, and relationship lists are cleared. Used by the fake agent (dry
// runs and tests); real agents write real content.
func Example(rel string, template []byte) (*Doc, error) {
	text := placeholderRe.ReplaceAllString(string(template), "Example")
	text = commentRe.ReplaceAllString(text, "")
	d, err := Parse(rel, []byte(text))
	if err != nil {
		return nil, err
	}
	for _, k := range RelKeys {
		if d.node(k) != nil {
			d.Set(k, []string{})
		}
	}
	for _, k := range dateKeys {
		if d.node(k) != nil && !IsDate(d.Str(k)) {
			d.Set(k, time.Now().Format("2006-01-02"))
		}
	}
	d.Body = strings.TrimSpace(blankRuns.ReplaceAllString(d.Body, "\n\n")) + "\n"
	return d, nil
}
