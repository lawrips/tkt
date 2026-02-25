package cli

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lawrips/tkt/internal/project"
)

func TestMigrateCentralAndBackToLocal(t *testing.T) {
	withWorkspace(t, func(dir string) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := project.Config{
			Projects: map[string]project.ProjectConfig{
				"demo": {
					Path:      project.DetectProjectPath(dir),
					Store:     "local",
					AutoLink:  true,
					AutoClose: true,
				},
			},
		}
		if err := project.Save(cfg); err != nil {
			t.Fatalf("save config: %v", err)
		}

		if err := os.WriteFile(filepath.Join(dir, ".tickets", "a.md"), []byte("---\nid: a\n---\n# A\n"), 0644); err != nil {
			t.Fatalf("seed local ticket: %v", err)
		}

		if _, _, err := runCmd(t, "yes\n", "migrate", "--central"); err != nil {
			t.Fatalf("migrate --central: %v", err)
		}
		if _, err := os.Stat(filepath.Join(home, ".tickets", "demo", "a.md")); err != nil {
			t.Fatalf("missing moved central ticket: %v", err)
		}

		loaded, err := project.Load()
		if err != nil {
			t.Fatalf("load config after central migrate: %v", err)
		}
		if loaded.Projects["demo"].Store != "central" {
			t.Fatalf("expected store central, got %s", loaded.Projects["demo"].Store)
		}

		if _, _, err := runCmd(t, "", "migrate", "--local", "--yes"); err != nil {
			t.Fatalf("migrate --local: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".tickets", "a.md")); err != nil {
			t.Fatalf("missing moved local ticket: %v", err)
		}

		loaded, err = project.Load()
		if err != nil {
			t.Fatalf("load config after local migrate: %v", err)
		}
		if loaded.Projects["demo"].Store != "local" {
			t.Fatalf("expected store local, got %s", loaded.Projects["demo"].Store)
		}
	})
}

func TestRecomputeBuildsCommitJournal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	withWorkspaceNoTickets(t, func(dir string) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := project.Config{
			Projects: map[string]project.ProjectConfig{
				"demo": {
					Path:      project.DetectProjectPath(dir),
					Store:     "local",
					AutoLink:  true,
					AutoClose: true,
				},
			},
		}
		if err := project.Save(cfg); err != nil {
			t.Fatalf("save config: %v", err)
		}

		runGit := func(args ...string) {
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
			}
		}

		runGit("init")
		runGit("config", "user.email", "tkt@example.com")
		runGit("config", "user.name", "tkt")

		writeAndCommit := func(name, message string) {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(name+"\n"), 0644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
			runGit("add", name)
			runGit("commit", "-m", message)
		}

		writeAndCommit("a.txt", "[abc-1] first commit")
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b.txt\n"), 0644); err != nil {
			t.Fatalf("write b.txt: %v", err)
		}
		runGit("add", "b.txt")
		runGit("commit", "-m", "recompute parse body", "-m", "fixes [abc-2]")

		if _, _, err := runCmd(t, "", "recompute", "--yes"); err != nil {
			t.Fatalf("recompute: %v", err)
		}

		journalPath := filepath.Join(home, ".tkt", "state", "demo", "commits.jsonl")
		raw, err := os.ReadFile(journalPath)
		if err != nil {
			t.Fatalf("read journal: %v", err)
		}

		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		entries := make([]map[string]any, 0)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var row map[string]any
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatalf("invalid journal line %q: %v", line, err)
			}
			entries = append(entries, row)
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("scan journal: %v", err)
		}

		if len(entries) < 2 {
			t.Fatalf("expected at least 2 journal entries, got %d", len(entries))
		}

		hasRef := false
		hasClose := false
		for _, row := range entries {
			ticketID, _ := row["ticket"].(string)
			action, _ := row["action"].(string)
			if ticketID == "abc-1" && action == "ref" {
				hasRef = true
				// First commit adds a.txt (1 line).
				filesRaw, _ := row["files_changed"].([]any)
				if len(filesRaw) != 1 {
					t.Fatalf("abc-1 ref: expected 1 file_changed, got %v", filesRaw)
				}
				foundA := false
				for _, file := range filesRaw {
					if name, _ := file.(string); name == "a.txt" {
						foundA = true
						break
					}
				}
				if !foundA {
					t.Fatalf("abc-1 ref: expected files_changed to contain a.txt, got %v", filesRaw)
				}
				if added, _ := row["lines_added"].(float64); added != 1 {
					t.Fatalf("abc-1 ref: expected lines_added=1, got %v", added)
				}
				if removedRaw, ok := row["lines_removed"]; ok {
					if removed, _ := removedRaw.(float64); removed != 0 {
						t.Fatalf("abc-1 ref: expected lines_removed=0, got %v", removedRaw)
					}
				}
				branch, _ := row["branch"].(string)
				if strings.TrimSpace(branch) == "" {
					t.Fatalf("abc-1 ref: expected non-empty branch, got %v", row["branch"])
				}
			}
			if ticketID == "abc-2" && action == "close" {
				hasClose = true
				// Second commit adds b.txt (1 line).
				filesRaw, _ := row["files_changed"].([]any)
				if len(filesRaw) != 1 {
					t.Fatalf("abc-2 close: expected 1 file_changed, got %v", filesRaw)
				}
				foundB := false
				for _, file := range filesRaw {
					if name, _ := file.(string); name == "b.txt" {
						foundB = true
						break
					}
				}
				if !foundB {
					t.Fatalf("abc-2 close: expected files_changed to contain b.txt, got %v", filesRaw)
				}
				if added, _ := row["lines_added"].(float64); added != 1 {
					t.Fatalf("abc-2 close: expected lines_added=1, got %v", added)
				}
				if removedRaw, ok := row["lines_removed"]; ok {
					if removed, _ := removedRaw.(float64); removed != 0 {
						t.Fatalf("abc-2 close: expected lines_removed=0, got %v", removedRaw)
					}
				}
				branch, _ := row["branch"].(string)
				if strings.TrimSpace(branch) == "" {
					t.Fatalf("abc-2 close: expected non-empty branch, got %v", row["branch"])
				}
				// Recompute should NOT set work_started or work_ended.
				if ws, ok := row["work_started"]; ok && ws != "" {
					t.Fatalf("abc-2 close: expected no work_started in recompute, got %v", ws)
				}
				if we, ok := row["work_ended"]; ok && we != "" {
					t.Fatalf("abc-2 close: expected no work_ended in recompute, got %v", we)
				}
				if ds, ok := row["duration_seconds"]; ok {
					if n, _ := ds.(float64); n != 0 {
						t.Fatalf("abc-2 close: expected no duration_seconds in recompute, got %v", ds)
					}
				}
			}
		}

		if !hasRef || !hasClose {
			t.Fatalf("expected ref/close entries, got %+v", entries)
		}
	})
}

func TestGetDiffStatsStableBranchSingleBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "tkt@example.com")
	runGit("config", "user.name", "tkt")
	runGit("checkout", "-b", "feature")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit("add", "a.txt")
	runGit("commit", "-m", "seed")

	shaOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	_, _, _, branch, err := getDiffStats(repo, sha)
	if err != nil {
		t.Fatalf("getDiffStats: %v", err)
	}
	if branch != "feature" {
		t.Fatalf("expected branch=feature, got %q", branch)
	}
}

func TestGetDiffStatsStableBranchAmbiguousReturnsEmpty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "tkt@example.com")
	runGit("config", "user.name", "tkt")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit("add", "a.txt")
	runGit("commit", "-m", "seed")

	shaOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaOut))

	// Create a second local branch pointing at the same commit.
	runGit("branch", "feature")

	_, _, _, branch, err := getDiffStats(repo, sha)
	if err != nil {
		t.Fatalf("getDiffStats: %v", err)
	}
	if branch != "" {
		t.Fatalf("expected empty branch for ambiguous commit, got %q", branch)
	}
}

func TestGetDiffStatsInvalidSHAReturnsError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "tkt@example.com")
	runGit("config", "user.name", "tkt")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit("add", "a.txt")
	runGit("commit", "-m", "seed")

	files, added, removed, branch, err := getDiffStats(repo, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil {
		t.Fatalf("expected error for invalid SHA, got nil")
	}
	if files != nil {
		t.Fatalf("expected nil files on error, got %v", files)
	}
	if added != 0 {
		t.Fatalf("expected added=0 on error, got %d", added)
	}
	if removed != 0 {
		t.Fatalf("expected removed=0 on error, got %d", removed)
	}
	if branch != "" {
		t.Fatalf("expected empty branch on error, got %q", branch)
	}
}
