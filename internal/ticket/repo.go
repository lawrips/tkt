package ticket

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultDir = ".tickets"

var (
	// ErrTicketNotFound indicates no matching ticket ID was found.
	ErrTicketNotFound = errors.New("ticket not found")
	// ErrAmbiguousID indicates multiple tickets matched a partial ID query.
	ErrAmbiguousID = errors.New("ticket id is ambiguous")
)

// Record is a parsed ticket file ready for command-level operations.
type Record struct {
	ID      string
	Path    string
	Front   Frontmatter
	Body    Body
	ModTime time.Time
}

// EnsureDir creates the ticket directory if it does not already exist.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// List loads all ticket markdown files in a directory.
func List(dir string) ([]Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Record{}, nil
		}
		return nil, err
	}

	paths := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(paths)

	out := make([]Record, 0, len(paths))
	for _, path := range paths {
		record, err := LoadRecord(path)
		if err != nil {
			// Skip unreadable/invalid ticket docs so one bad file does not
			// break listing the full repository.
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

// LoadRecord reads one ticket from disk.
func LoadRecord(path string) (Record, error) {
	doc, err := Load(path)
	if err != nil {
		return Record{}, err
	}

	body := ParseBody(doc.Body)
	id := doc.Frontmatter.ID
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	var modTime time.Time
	if info, err := os.Stat(path); err == nil {
		modTime = info.ModTime()
	}

	return Record{
		ID:      id,
		Path:    path,
		Front:   doc.Frontmatter,
		Body:    body,
		ModTime: modTime,
	}, nil
}

// SaveRecord writes a ticket back to disk.
func SaveRecord(record Record) error {
	doc := Document{
		Frontmatter: record.Front,
		Body:        RenderBody(record.Body),
	}
	return Save(record.Path, doc)
}

// ResolvePath resolves an exact or partial ticket ID to a single file path.
func ResolvePath(dir string, query string) (string, error) {
	query = strings.TrimSpace(strings.TrimSuffix(query, ".md"))
	if query == "" {
		return "", ErrTicketNotFound
	}

	exactPath := filepath.Join(dir, query+".md")
	if _, err := os.Stat(exactPath); err == nil {
		return exactPath, nil
	}

	records, err := List(dir)
	if err != nil {
		return "", err
	}

	matches := make([]string, 0)
	for _, record := range records {
		id := record.ID
		if strings.HasPrefix(id, query) || strings.Contains(id, query) {
			matches = append(matches, record.Path)
		}
	}

	if len(matches) == 0 {
		return "", ErrTicketNotFound
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, path := range matches {
			ids = append(ids, strings.TrimSuffix(filepath.Base(path), ".md"))
		}
		sort.Strings(ids)
		return "", fmt.Errorf("%w: %s", ErrAmbiguousID, strings.Join(ids, ", "))
	}

	return matches[0], nil
}

// LoadByID resolves then loads a ticket by exact or partial ID.
func LoadByID(dir, query string) (Record, error) {
	path, err := ResolvePath(dir, query)
	if err != nil {
		return Record{}, err
	}
	return LoadRecord(path)
}
