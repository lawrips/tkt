package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := Config{
		Projects: map[string]ProjectConfig{
			"alpha": {
				Path:      "/tmp/alpha",
				Store:     "central",
				AutoLink:  true,
				AutoClose: false,
			},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	project, ok := loaded.Projects["alpha"]
	if !ok {
		t.Fatalf("missing project alpha in loaded config")
	}
	if project.Path != "/tmp/alpha" || project.Store != "central" || !project.AutoLink || project.AutoClose {
		t.Fatalf("unexpected round-trip project: %+v", project)
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(home, ".tkt") {
		t.Fatalf("unexpected config path %s", path)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(cfg.Projects) != 0 {
		t.Fatalf("expected empty projects, got %+v", cfg.Projects)
	}
}

func TestResolvePrecedence(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	cfg := Config{
		Projects: map[string]ProjectConfig{
			"mapped": {
				Path:      cwd,
				Store:     "local",
				AutoLink:  true,
				AutoClose: true,
			},
		},
	}

	name, source := ResolveName(cfg, cwd, "flag-override")
	if name != "flag-override" || source != "flag" {
		t.Fatalf("flag should win, got %s (%s)", name, source)
	}

	name, source = ResolveName(cfg, cwd, "")
	if name != "mapped" || source != "config" {
		t.Fatalf("config path should win, got %s (%s)", name, source)
	}

	name, source = ResolveName(Config{Projects: map[string]ProjectConfig{}}, cwd, "")
	if name != "repo" || source != "dirname" {
		t.Fatalf("dirname fallback mismatch, got %s (%s)", name, source)
	}
}

func TestResolveUsesGitRemoteWhenNoConfigMatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}
	run("init")
	run("remote", "add", "origin", "git@github.com:lawrence/ticket-v2.git")

	name, source := ResolveName(Config{Projects: map[string]ProjectConfig{}}, repo, "")
	if name != "ticket-v2" || source != "git_remote" {
		t.Fatalf("expected git remote resolution, got %s (%s)", name, source)
	}
}
