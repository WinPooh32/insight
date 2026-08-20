package index

import (
	"strings"
)

// indexEntry is one parsed bullet of index.md.
type indexEntry struct {
	title       string
	path        string
	description string
}

// section is one markdown section of a research doc: the heading
// line's text ("" for the text before the first heading) and the
// body up to the next heading.
type section struct {
	heading string
	body    string
}

// parseIndex extracts the "- [title](path) — description" bullets
// from index.md. A description may wrap onto following indented
// lines; wrapped lines are joined with a single space. Any other
// line (headings, plain bullets, …) is not part of an entry.
func parseIndex(data []byte) []indexEntry {
	var (
		entries []indexEntry
		current *indexEntry
	)

	for line := range strings.Lines(string(data)) {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "- ") {
			if e, ok := parseBullet(trimmed); ok {
				entries = append(entries, e)
				current = &entries[len(entries)-1]
			} else {
				current = nil
			}

			continue
		}

		// A blank line ends the current entry's wrapped
		// description; an indented line continues it.
		if trimmed == "" {
			current = nil

			continue
		}

		if !strings.HasPrefix(line, " ") || current == nil {
			continue
		}

		if current.description == "" {
			current.description = trimmed
		} else {
			current.description += " " + trimmed
		}
	}

	return entries
}

// parseBullet parses one "- [title](path) — description" line. The
// separator is an em dash (the index.md format) or "--"; a bullet
// without a separator is still an entry with an empty description.
func parseBullet(line string) (indexEntry, bool) {
	rest := strings.TrimPrefix(line, "- ")

	open := strings.Index(rest, "](")
	if open <= 0 || !strings.HasPrefix(rest, "[") {
		return indexEntry{title: "", path: "", description: ""}, false
	}

	title := strings.TrimSpace(rest[1:open])
	rest = rest[open+2:]

	closeIdx := strings.Index(rest, ")")
	if closeIdx < 0 {
		return indexEntry{title: "", path: "", description: ""}, false
	}

	path := rest[:closeIdx]
	rest = strings.TrimSpace(rest[closeIdx+1:])

	description := ""

	switch {
	case strings.HasPrefix(rest, "—"):
		description = strings.TrimSpace(rest[len("—"):])
	case strings.HasPrefix(rest, "--"):
		description = strings.TrimSpace(rest[len("--"):])
	}

	if title == "" || path == "" {
		return indexEntry{title: "", path: "", description: ""}, false
	}

	return indexEntry{title: title, path: path, description: description}, true
}

// chunkSections splits a research doc on heading lines (any line
// starting with "#"). Each section is the heading text plus the body
// lines up to the next heading; text before the first heading is a
// section with an empty heading. Empty sections (no non-whitespace
// body and no heading) are skipped.
func chunkSections(data []byte) []section {
	var (
		sections []section
		cur      section
	)

	flush := func() {
		body := strings.TrimSpace(cur.body)
		if body != "" || cur.heading != "" {
			sections = append(sections, section{heading: cur.heading, body: body})
		}

		cur = section{heading: "", body: ""}
	}

	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, "#") {
			flush()

			cur.heading = strings.TrimSpace(strings.TrimLeft(line, "#"))
		} else {
			cur.body += line + "\n"
		}
	}

	flush()

	return sections
}
