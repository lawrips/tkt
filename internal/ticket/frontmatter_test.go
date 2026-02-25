package ticket

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripPreservesKnownAndUnknownFrontmatter(t *testing.T) {
	raw := mustReadFixture(t, "task_with_custom.md")

	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if doc.Frontmatter.ID != "pa-5c46" {
		t.Fatalf("unexpected id: %s", doc.Frontmatter.ID)
	}

	custom, ok := doc.Frontmatter.Extra["custom_field"]
	if !ok {
		t.Fatalf("custom_field missing from extra frontmatter")
	}
	if custom.Raw != "keep-me" {
		t.Fatalf("custom_field changed: %#v", custom)
	}

	out, err := Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !bytes.Contains(out, []byte("custom_field: keep-me")) {
		t.Fatalf("custom_field missing from marshaled output:\n%s", string(out))
	}

	reparsed, err := Parse(out)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}

	if reparsed.Frontmatter.ID != doc.Frontmatter.ID {
		t.Fatalf("id changed after round trip: %s != %s", reparsed.Frontmatter.ID, doc.Frontmatter.ID)
	}
	if reparsed.Body != doc.Body {
		t.Fatalf("body changed during round trip")
	}
}

func TestLoadAndSave(t *testing.T) {
	raw := mustReadFixture(t, "epic.md")

	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "ticket.md")
	if err := Save(tmp, doc); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded, err := Load(tmp)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if reloaded.Frontmatter.ID != "pa-ff45" {
		t.Fatalf("unexpected reloaded id: %s", reloaded.Frontmatter.ID)
	}
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "testdata", "tickets", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}
