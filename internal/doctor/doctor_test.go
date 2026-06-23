package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestRunReportsHealthyLocalProject(t *testing.T) {
	home, repo, ticketDir := setupDoctorProject(t, true)
	if err := os.MkdirAll(filepath.Join(home, ".tkt", "state"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".tkt", "workflow.md"), []byte("Use tkt workflow for tickets.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("Run tkt workflow before ticket work.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeDoctorTicket(t, ticketDir, "c-one", "One")

	report := Run(Options{CWD: repo, Now: fixedNow})

	if report.Summary.Fail != 0 {
		t.Fatalf("expected no failures, got %#v", report.Checks)
	}
	if report.Project.Name != "demo" || !report.Project.Registered {
		t.Fatalf("unexpected project summary: %#v", report.Project)
	}
	if checkStatus(report, "project.ticket_parse") != StatusPass {
		t.Fatalf("expected parse pass, got %#v", report.Checks)
	}
	if checkStatus(report, "agent.instructions") != StatusPass {
		t.Fatalf("expected agent instructions pass, got %#v", report.Checks)
	}
}

func TestRunReportsWarningsForDefaultWorkflowAndParseDiagnostics(t *testing.T) {
	_, repo, ticketDir := setupDoctorProject(t, true)
	if err := os.WriteFile(filepath.Join(ticketDir, "broken.md"), []byte("# missing frontmatter\n"), 0644); err != nil {
		t.Fatal(err)
	}

	report := Run(Options{CWD: repo, Now: fixedNow, AgentInstructionCandidates: []string{filepath.Join(repo, "missing.md")}})

	if checkStatus(report, "global.workflow") != StatusWarn {
		t.Fatalf("expected workflow warning, got %#v", report.Checks)
	}
	if checkStatus(report, "project.ticket_parse") != StatusWarn {
		t.Fatalf("expected parse warning, got %#v", report.Checks)
	}
	if report.Status != StatusWarn {
		t.Fatalf("expected overall warn, got %s", report.Status)
	}
}

func TestRunReportsFailureForUnregisteredProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := t.TempDir()

	report := Run(Options{CWD: repo, Now: fixedNow, AgentInstructionCandidates: []string{}})

	if checkStatus(report, "project.registration") != StatusFail {
		t.Fatalf("expected unregistered project failure, got %#v", report.Checks)
	}
	if report.Status != StatusFail {
		t.Fatalf("expected overall fail, got %s", report.Status)
	}
}

func TestFormatTextGroupsChecksAndCommands(t *testing.T) {
	report := Report{
		Status:  StatusWarn,
		Summary: Summary{Pass: 1, Warn: 1, Fail: 0, Total: 2, Worst: StatusWarn},
		Checks: []Check{
			{ID: "project.registration", Category: "project", Status: StatusWarn, Message: "Project is not registered.", Command: "tkt init"},
			{ID: "global.config", Category: "global", Status: StatusPass, Message: "Config loaded."},
		},
	}

	out := FormatText(report)
	for _, want := range []string{"TKT Doctor", "Overall: WARN", "Global:", "Project:", "PASS Config loaded.", "command: tkt init"} {
		if !strings.Contains(out, want) {
			t.Fatalf("format missing %q:\n%s", want, out)
		}
	}
}

func setupDoctorProject(t *testing.T, local bool) (home, repo, ticketDir string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	repo = t.TempDir()
	if local {
		ticketDir = filepath.Join(repo, ".tickets")
	} else {
		ticketDir = filepath.Join(home, ".tickets", "demo")
	}
	if err := os.MkdirAll(ticketDir, 0755); err != nil {
		t.Fatal(err)
	}
	store := "local"
	if !local {
		store = "central"
	}
	cfg := project.Config{Projects: map[string]project.ProjectConfig{
		"demo": {
			Path:         repo,
			Store:        store,
			RegisteredAt: time.Now().UTC().Format(time.RFC3339),
		},
	}}
	if err := project.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return home, repo, ticketDir
}

func writeDoctorTicket(t *testing.T, dir, id, title string) {
	t.Helper()
	record := ticket.Record{
		ID:   id,
		Path: filepath.Join(dir, id+".md"),
		Front: ticket.Frontmatter{
			ID:       id,
			Status:   "open",
			Deps:     []string{},
			Links:    []string{},
			Created:  time.Now().UTC().Format(time.RFC3339),
			Type:     "task",
			Priority: 2,
			Extra:    map[string]ticket.ExtraField{},
		},
		Body: ticket.Body{Title: title},
	}
	if err := ticket.SaveRecord(record); err != nil {
		t.Fatal(err)
	}
}

func checkStatus(report Report, id string) Status {
	for _, check := range report.Checks {
		if check.ID == id {
			return check.Status
		}
	}
	return ""
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
}
