package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if len(token) < 32 {
		t.Fatalf("token too short: %q", token)
	}
}

func TestAPITokenRequired(t *testing.T) {
	server, err := New(Options{Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unauthorized") {
		t.Fatalf("expected unauthorized body, got %s", rec.Body.String())
	}
}

func TestAPIHeaderTokenAccepted(t *testing.T) {
	server, err := New(Options{Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServesEmbeddedAppAssets(t *testing.T) {
	server, err := New(Options{Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/", "/assets/styles.css", "/assets/app.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s expected ok, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestEmbeddedAppDoesNotLoadExternalAssets(t *testing.T) {
	index, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, disallowed := range []string{"https://", "http://", "//cdn", "fonts.googleapis"} {
		if strings.Contains(string(index), disallowed) {
			t.Fatalf("index contains external asset reference %q", disallowed)
		}
	}
}

func TestEmbeddedAppIncludesEditingControls(t *testing.T) {
	appJS, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"edit-form", "note-form", "dep-input", "link-input", "stale_revision"} {
		if !strings.Contains(string(appJS), required) {
			t.Fatalf("app.js missing editing marker %q", required)
		}
	}
}

func TestTicketListReturnsParseDiagnostics(t *testing.T) {
	_, repo, ticketDir := setupWebProject(t)
	writeWebTicket(t, ticketDir, "c-good", "Good")
	if err := os.WriteFile(filepath.Join(ticketDir, "broken.md"), []byte("# nope\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{Token: "secret", CWD: repo})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/demo/tickets", nil)
	req.Header.Set("X-TKT-Token", "secret")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items       []map[string]any `json:"items"`
		Diagnostics []map[string]any `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one item, got %#v", payload.Items)
	}
	if len(payload.Diagnostics) != 1 || payload.Diagnostics[0]["code"] != "missing_frontmatter" {
		t.Fatalf("expected parse diagnostic, got %#v", payload.Diagnostics)
	}
}

func TestPatchTicketRejectsStaleRevision(t *testing.T) {
	_, repo, ticketDir := setupWebProject(t)
	writeWebTicket(t, ticketDir, "c-one", "Original")
	server, err := New(Options{Token: "secret", CWD: repo})
	if err != nil {
		t.Fatal(err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/projects/demo/tickets/c-one", nil)
	getReq.Header.Set("Authorization", "Bearer secret")
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("detail failed: %d %s", getRec.Code, getRec.Body.String())
	}
	var detail struct {
		Revision map[string]string `json:"revision"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}

	record, err := ticket.LoadByID(ticketDir, "c-one")
	if err != nil {
		t.Fatal(err)
	}
	record.Body.Title = "Changed elsewhere"
	if err := ticket.SaveRecord(record); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"source":"test","revision":{"hash":"` + detail.Revision["hash"] + `","mod_time":"` + detail.Revision["mod_time"] + `"},"fields":{"title":"Web edit"}}`)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/projects/demo/tickets/c-one", bytes.NewReader(body))
	patchReq.Header.Set("Authorization", "Bearer secret")
	patchReq.Header.Set("Content-Type", "application/json")
	patchRec := httptest.NewRecorder()
	server.ServeHTTP(patchRec, patchReq)

	if patchRec.Code != http.StatusConflict {
		t.Fatalf("expected stale conflict, got %d: %s", patchRec.Code, patchRec.Body.String())
	}
	if !strings.Contains(patchRec.Body.String(), "stale_revision") {
		t.Fatalf("expected stale_revision error, got %s", patchRec.Body.String())
	}
}

func TestPathTraversalRejected(t *testing.T) {
	_, repo, _ := setupWebProject(t)
	server, err := New(Options{Token: "secret", CWD: repo})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/projects/demo/tickets/..%2Fsecret", nil)
	req.Header.Set("X-TKT-Token", "secret")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnexpectedOriginRejectedForMutation(t *testing.T) {
	_, repo, ticketDir := setupWebProject(t)
	writeWebTicket(t, ticketDir, "c-one", "Original")
	server, err := New(Options{Token: "secret", CWD: repo})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/projects/demo/tickets/c-one", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-TKT-Token", "secret")
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
}

func setupWebProject(t *testing.T) (home, repo, ticketDir string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	repo = t.TempDir()
	ticketDir = filepath.Join(repo, ".tickets")
	if err := os.MkdirAll(ticketDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := project.Config{Projects: map[string]project.ProjectConfig{
		"demo": {
			Path:         repo,
			Store:        "local",
			AutoLink:     true,
			AutoClose:    true,
			RegisteredAt: time.Now().UTC().Format(time.RFC3339),
		},
	}}
	if err := project.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return home, repo, ticketDir
}

func writeWebTicket(t *testing.T, dir, id, title string) {
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
		t.Fatalf("save ticket: %v", err)
	}
}
