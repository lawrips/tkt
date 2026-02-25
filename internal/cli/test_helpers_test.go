package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

var cwdMu sync.Mutex

func withWorkspace(t *testing.T, fn func(dir string)) {
	t.Helper()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tickets"), 0755); err != nil {
		t.Fatalf("mkdir .tickets: %v", err)
	}

	cwdMu.Lock()
	defer cwdMu.Unlock()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore chdir: %v", err)
		}
	}()

	// Register temp dir as a project so the init guard passes.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := project.Config{
		Projects: map[string]project.ProjectConfig{
			"test": {Path: dir, Store: "local"},
		},
	}
	if err := project.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	fn(dir)
}

func runCmd(t *testing.T, stdin string, args ...string) (stdout string, stderr string, err error) {
	t.Helper()

	var out bytes.Buffer
	var errOut bytes.Buffer
	err = runWithIO(args, strings.NewReader(stdin), &out, &errOut)
	return out.String(), errOut.String(), err
}

func seedTicket(t *testing.T, id string, front ticket.Frontmatter, body ticket.Body) {
	t.Helper()

	if front.ID == "" {
		front.ID = id
	}
	if front.Deps == nil {
		front.Deps = []string{}
	}
	if front.Links == nil {
		front.Links = []string{}
	}
	if front.Extra == nil {
		front.Extra = map[string]ticket.ExtraField{}
	}

	record := ticket.Record{
		ID:    id,
		Path:  filepath.Join(ticket.DefaultDir, id+".md"),
		Front: front,
		Body:  body,
	}

	if err := ticket.SaveRecord(record); err != nil {
		t.Fatalf("seed ticket %s: %v", id, err)
	}
}
