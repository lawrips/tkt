package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateUsesCentralStoreAfterInitConfig(t *testing.T) {
	withWorkspaceNoTickets(t, func(_ string) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		if _, _, err := runCmd(t, "", "init", "--project", "demo", "--store", "central", "--yes"); err != nil {
			t.Fatalf("init central: %v", err)
		}

		if _, _, err := runCmd(t, "", "create", "Central ticket", "--id", "central-create-1"); err != nil {
			t.Fatalf("create: %v", err)
		}

		centralPath := filepath.Join(home, ".tickets", "demo", "central-create-1.md")
		if _, err := os.Stat(centralPath); err != nil {
			t.Fatalf("expected created ticket in central store: %v", err)
		}

		if _, err := os.Stat(filepath.Join(".tickets", "central-create-1.md")); !os.IsNotExist(err) {
			t.Fatalf("did not expect local .tickets file for central project, stat err=%v", err)
		}
	})
}

func TestCreateRequiresInitWithoutConfig(t *testing.T) {
	withWorkspaceNoTickets(t, func(_ string) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		_, _, err := runCmd(t, "", "create", "Should fail", "--id", "no-init-1")
		if err == nil {
			t.Fatalf("expected init guard error, got nil")
		}
		if !strings.Contains(err.Error(), "tkt not initialized") {
			t.Fatalf("expected init guard message, got: %v", err)
		}
	})
}
