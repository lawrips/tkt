package ticket

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const delimiter = "---\n"

var (
	// ErrMissingFrontmatter indicates a markdown ticket is missing YAML frontmatter.
	ErrMissingFrontmatter = errors.New("ticket file missing YAML frontmatter")
	// ErrMalformedFrontmatter indicates opening/closing frontmatter delimiters are invalid.
	ErrMalformedFrontmatter = errors.New("ticket frontmatter delimiters are malformed")
)

// Parse parses a ticket markdown file into structured frontmatter + markdown body.
func Parse(raw []byte) (Document, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")

	frontmatter, body, err := splitFrontmatter(text)
	if err != nil {
		return Document{}, err
	}

	fm, err := parseFrontmatter(frontmatter)
	if err != nil {
		return Document{}, err
	}

	return Document{
		Frontmatter: fm,
		Body:        body,
	}, nil
}

// Marshal serializes a parsed ticket back into markdown format.
func Marshal(doc Document) ([]byte, error) {
	fm, err := marshalFrontmatter(doc.Frontmatter)
	if err != nil {
		return nil, err
	}

	return []byte(delimiter + fm + delimiter + doc.Body), nil
}

// Load reads and parses a ticket markdown file from disk.
func Load(path string) (Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	return Parse(raw)
}

// Save marshals and writes a ticket markdown file to disk.
func Save(path string, doc Document) error {
	raw, err := Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func splitFrontmatter(markdown string) (frontmatter string, body string, err error) {
	if !strings.HasPrefix(markdown, delimiter) {
		return "", "", ErrMissingFrontmatter
	}

	rest := markdown[len(delimiter):]
	idx := strings.Index(rest, "\n"+delimiter)
	if idx < 0 {
		return "", "", ErrMalformedFrontmatter
	}

	frontmatter = rest[:idx]
	body = rest[idx+len("\n"+delimiter):]
	return frontmatter, body, nil
}

func parseFrontmatter(raw string) (Frontmatter, error) {
	lines := strings.Split(raw, "\n")
	fm := Frontmatter{
		Extra: map[string]ExtraField{},
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if countIndent(line) != 0 {
			return Frontmatter{}, fmt.Errorf("invalid top-level indentation in frontmatter line: %q", line)
		}

		key, value, ok := splitKeyValue(line)
		if !ok {
			return Frontmatter{}, fmt.Errorf("invalid frontmatter line: %q", line)
		}

		if value == "" {
			j := i + 1
			blockLines := make([]string, 0)
			for ; j < len(lines); j++ {
				next := lines[j]
				if strings.TrimSpace(next) == "" {
					blockLines = append(blockLines, next)
					continue
				}
				if countIndent(next) == 0 && isKeyLine(next) {
					break
				}
				blockLines = append(blockLines, next)
			}
			if err := assignFrontmatterField(&fm, key, "", strings.Join(blockLines, "\n"), true); err != nil {
				return Frontmatter{}, err
			}
			i = j - 1
			continue
		}

		if err := assignFrontmatterField(&fm, key, value, "", false); err != nil {
			return Frontmatter{}, err
		}
	}

	return fm, nil
}

func assignFrontmatterField(fm *Frontmatter, key, value, blockRaw string, isBlock bool) error {
	switch key {
	case "id":
		fm.ID = unquote(value)
	case "status":
		fm.Status = unquote(value)
	case "deps":
		list, err := parseInlineList(value)
		if err != nil {
			return fmt.Errorf("parse deps: %w", err)
		}
		fm.Deps = list
	case "links":
		list, err := parseInlineList(value)
		if err != nil {
			return fmt.Errorf("parse links: %w", err)
		}
		fm.Links = list
	case "created":
		fm.Created = unquote(value)
	case "type":
		fm.Type = unquote(value)
	case "priority":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("parse priority: %w", err)
		}
		fm.Priority = n
	case "assignee":
		fm.Assignee = unquote(value)
	case "parent":
		fm.Parent = unquote(value)
	case "tags":
		list, err := parseInlineList(value)
		if err != nil {
			return fmt.Errorf("parse tags: %w", err)
		}
		fm.Tags = list
	case "external_ref":
		fm.ExternalRef = unquote(value)
	default:
		if fm.Extra == nil {
			fm.Extra = map[string]ExtraField{}
		}
		if isBlock {
			fm.Extra[key] = ExtraField{Raw: blockRaw, Block: true}
		} else {
			fm.Extra[key] = ExtraField{Raw: value, Block: false}
		}
	}
	return nil
}

func marshalFrontmatter(fm Frontmatter) (string, error) {
	lines := make([]string, 0, 16)

	if fm.ID != "" {
		lines = append(lines, "id: "+fm.ID)
	}
	if fm.Status != "" {
		lines = append(lines, "status: "+fm.Status)
	}
	if fm.Deps != nil {
		lines = append(lines, "deps: "+formatInlineList(fm.Deps))
	}
	if fm.Links != nil {
		lines = append(lines, "links: "+formatInlineList(fm.Links))
	}
	if fm.Created != "" {
		lines = append(lines, "created: "+fm.Created)
	}
	if fm.Type != "" {
		lines = append(lines, "type: "+fm.Type)
	}
	lines = append(lines, fmt.Sprintf("priority: %d", fm.Priority))
	if fm.Assignee != "" {
		lines = append(lines, "assignee: "+fm.Assignee)
	}
	if fm.Parent != "" {
		lines = append(lines, "parent: "+fm.Parent)
	}
	if fm.Tags != nil {
		lines = append(lines, "tags: "+formatInlineList(fm.Tags))
	}
	if fm.ExternalRef != "" {
		lines = append(lines, "external_ref: "+fm.ExternalRef)
	}

	if len(fm.Extra) > 0 {
		keys := make([]string, 0, len(fm.Extra))
		for key := range fm.Extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			extra := fm.Extra[key]
			if extra.Block {
				lines = append(lines, key+":")
				if extra.Raw != "" {
					lines = append(lines, extra.Raw)
				}
				continue
			}
			value := strings.TrimSpace(extra.Raw)
			if value == "" {
				lines = append(lines, key+":")
			} else {
				lines = append(lines, key+": "+value)
			}
		}
	}

	return strings.Join(lines, "\n") + "\n", nil
}

func parseInlineList(value string) ([]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "[]" {
		return []string{}, nil
	}
	if trimmed == "" {
		return []string{}, nil
	}
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return nil, fmt.Errorf("invalid list literal %q", value)
	}

	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return []string{}, nil
	}

	parts := strings.Split(inner, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		items = append(items, unquote(item))
	}
	return items, nil
}

func formatInlineList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func splitKeyValue(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	return key, value, true
}

func isKeyLine(line string) bool {
	_, _, ok := splitKeyValue(line)
	return ok
}

func countIndent(line string) int {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return i
}

func unquote(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 {
		return trimmed
	}
	if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') ||
		(trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
		return trimmed[1 : len(trimmed)-1]
	}
	return trimmed
}
