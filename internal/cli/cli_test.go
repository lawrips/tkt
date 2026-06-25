package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpListsV1Commands(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	if err := Run([]string{"--help"}, &out, &errOut); err != nil {
		t.Fatalf("run help: %v", err)
	}

	help := out.String()
	for _, required := range []string{
		"show",
		"ls",
		"ready",
		"blocked",
		"closed",
		"create",
		"edit",
		"add-note",
		"delete",
		"dep",
		"undep",
		"link",
		"unlink",
		"query",
		"stats",
		"timeline",
		"workflow",
		"doctor",
		"lifecycle",
		"progress",
		"dashboard",
		"epic-view",
		"tui",
		"serve",
		"web",
		"mcp",
		"config",
		"init",
		"migrate",
		"recompute",
		"version",
	} {
		if !strings.Contains(help, required) {
			t.Fatalf("missing command in help output: %s\n%s", required, help)
		}
	}
}

func TestHelpCreateShowsFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	if err := Run([]string{"help", "create"}, &out, &errOut); err != nil {
		t.Fatalf("run help create: %v", err)
	}

	help := out.String()
	for _, required := range []string{
		"--id",
		"--type",
		"--priority",
		"--parent",
		"--tags",
		"Examples:",
	} {
		if !strings.Contains(help, required) {
			t.Fatalf("missing in help create output: %s\n%s", required, help)
		}
	}
}

func TestDepTreeErrorsWhenTicketMissing(t *testing.T) {
	withWorkspace(t, func(_ string) {
		var out bytes.Buffer
		var errOut bytes.Buffer

		err := Run([]string{"dep", "tree", "pa-5c46"}, &out, &errOut)
		if err == nil {
			t.Fatalf("expected missing ticket error")
		}
		if !strings.Contains(err.Error(), "ticket not found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRequiresInit(t *testing.T) {
	// Commands that should NOT require init
	noInit := []string{"init", "config", "tui", "mcp", "serve", "web", "workflow", "doctor", "version"}
	for _, cmd := range noInit {
		if requiresInit(cmd) {
			t.Errorf("requiresInit(%q) = true, want false", cmd)
		}
	}

	// Commands that SHOULD require init
	needsInit := []string{"show", "ls", "create", "edit", "delete", "dep", "stats", "dashboard"}
	for _, cmd := range needsInit {
		if !requiresInit(cmd) {
			t.Errorf("requiresInit(%q) = false, want true", cmd)
		}
	}
}

func TestDoctorSkipsInitCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()

	cwdMu.Lock()
	defer cwdMu.Unlock()
	original, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(original)

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Run([]string{"doctor"}, &out, &errOut)
	if err != nil {
		t.Fatalf("doctor should not hit init guard: %v", err)
	}
	if !strings.Contains(out.String(), "TKT Doctor") {
		t.Fatalf("unexpected doctor output: %s", out.String())
	}
}

func TestServeSubcommandsSkipInitCheck(t *testing.T) {
	// Simulate a fresh system: no projects configured.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// serve start/stop/status/logs should not fail with "not initialized"
	for _, sub := range []string{"stop", "status", "logs"} {
		var out bytes.Buffer
		var errOut bytes.Buffer
		err := Run([]string{"serve", sub}, &out, &errOut)
		// These may return non-init errors (e.g. "not running"), but must NOT
		// return the init guard error.
		if err != nil && strings.Contains(err.Error(), "not initialized") {
			t.Errorf("serve %s hit init guard: %v", sub, err)
		}
	}
}

func TestWebSubcommandsSkipInitCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, sub := range []string{"stop", "status", "logs"} {
		var out bytes.Buffer
		var errOut bytes.Buffer
		err := Run([]string{"web", sub}, &out, &errOut)
		if err != nil && strings.Contains(err.Error(), "not initialized") {
			t.Errorf("web %s hit init guard: %v", sub, err)
		}
	}
}

func TestWebStatePathsSeparateFromServe(t *testing.T) {
	webPID, err := webPIDPath()
	if err != nil {
		t.Fatal(err)
	}
	servePID, err := servePIDPath()
	if err != nil {
		t.Fatal(err)
	}
	if webPID == servePID {
		t.Fatalf("web pid path should differ from serve pid path")
	}
}

func TestWriteWebStateUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.json")
	err := writeWebState(path, webState{PID: 123, URL: "http://127.0.0.1:1/?token=secret", Token: "secret"})
	if err != nil {
		t.Fatalf("write web state: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat web state: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("web state permissions = %v, want 0600", got)
	}
}

func TestWebStatusJSONOmitsStaleURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	pidPath, err := webPIDPath()
	if err != nil {
		t.Fatal(err)
	}
	statePath, err := webStatePath()
	if err != nil {
		t.Fatal(err)
	}
	const stalePID = 0
	if err := writePIDFile(pidPath, stalePID); err != nil {
		t.Fatal(err)
	}
	if err := writeWebState(statePath, webState{
		PID:   stalePID,
		URL:   "http://127.0.0.1:7420/?token=secret",
		Token: "secret",
	}); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCmd(t, "", "--json", "web", "status")
	if err != nil {
		t.Fatalf("web status: %v", err)
	}
	var envelope struct {
		Data struct {
			Running bool   `json:"running"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("parse status json: %v output=%q", err, out)
	}
	if envelope.Data.Running {
		t.Fatalf("expected stale pid to be reported not running")
	}
	if envelope.Data.URL != "" {
		t.Fatalf("expected stale token URL to be omitted, got %q", envelope.Data.URL)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale state file removed, stat err=%v", err)
	}
}

func TestInitGuardBlocksCommandsWithoutProject(t *testing.T) {
	// Simulate a fresh system: no projects configured.
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	cwdMu.Lock()
	defer cwdMu.Unlock()
	original, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(original)

	// Commands that need init should fail with "not initialized"
	for _, cmd := range []string{"ls", "stats", "dashboard"} {
		var out bytes.Buffer
		var errOut bytes.Buffer
		err := Run([]string{cmd}, &out, &errOut)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Errorf("%s should fail with init error, got: %v", cmd, err)
		}
	}
}

func TestAliasResolves(t *testing.T) {
	withWorkspace(t, func(_ string) {
		var out bytes.Buffer
		var errOut bytes.Buffer

		if err := Run([]string{"list"}, &out, &errOut); err != nil {
			t.Fatalf("run list alias: %v", err)
		}

		got := out.String()
		if !strings.Contains(got, "No tickets.") {
			t.Fatalf("expected list output, got %q", got)
		}
	})
}
