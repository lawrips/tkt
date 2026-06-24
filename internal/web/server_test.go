package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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

func TestURLForIPv6Loopback(t *testing.T) {
	server, err := New(Options{Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	listener := fakeListener{addr: &net.TCPAddr{IP: net.ParseIP("::1"), Port: 4321}}

	got := server.URLFor(listener)

	if got != "http://[::1]:4321/?token=secret" {
		t.Fatalf("unexpected IPv6 URL: %s", got)
	}
}

func TestNormalizeAddrUsesStableDefault(t *testing.T) {
	if got := NormalizeAddr(""); got != DefaultAddr {
		t.Fatalf("NormalizeAddr empty = %q, want %q", got, DefaultAddr)
	}
	if got := NormalizeAddr(" 127.0.0.1:0 "); got != "127.0.0.1:0" {
		t.Fatalf("NormalizeAddr explicit = %q", got)
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

func TestAPIInvalidTokenRejected(t *testing.T) {
	server, err := New(Options{Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", rec.Code)
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

func TestPrivilegedAPIRoutesAreAbsent(t *testing.T) {
	server, err := New(Options{Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/shell", "/api/git/push", "/api/git/pull", "/api/git/fetch", "/api/serve/start", "/api/serve/stop"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s expected not found, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestServesEmbeddedAppAssets(t *testing.T) {
	server, err := New(Options{Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/", "/assets/styles.css", "/assets/app.js", "/assets/markdown.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s expected ok, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestEmbeddedAppDoesNotLoadExternalAssets(t *testing.T) {
	for _, path := range []string{"assets/index.html", "assets/styles.css", "assets/app.js", "assets/markdown.js"} {
		raw, err := assets.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, disallowed := range []string{"https://", "http://", "//cdn", "fonts.googleapis", "@import"} {
			if strings.Contains(string(raw), disallowed) {
				t.Fatalf("%s contains external asset reference %q", path, disallowed)
			}
		}
	}
}

func TestEmbeddedStylesUseSemanticColorTokens(t *testing.T) {
	raw, err := assets.ReadFile("assets/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	for _, required := range []string{
		"--semantic-shell-",
		"--semantic-pane-",
		"--semantic-list-",
		"--semantic-content-",
		"--semantic-detail-",
		"--semantic-control-",
		"--semantic-chip-",
	} {
		if !strings.Contains(css, required) {
			t.Fatalf("styles.css missing semantic token family %q", required)
		}
	}

	rootEnd := strings.Index(css, "\n}\n\n* {")
	if rootEnd == -1 {
		t.Fatal("styles.css root token block not found")
	}
	componentCSS := css[rootEnd:]
	if strings.Contains(componentCSS, "var(--raw-") {
		t.Fatal("component CSS references raw palette tokens")
	}
	if regexp.MustCompile(`#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})\b`).MatchString(componentCSS) {
		t.Fatal("component CSS contains a raw hex color")
	}
	for _, rawColor := range []string{"rgba(", "rgb(", "hsla(", "hsl("} {
		if strings.Contains(componentCSS, rawColor) {
			t.Fatalf("component CSS contains raw color %q", rawColor)
		}
	}
}

func TestEmbeddedAppIncludesEditingControls(t *testing.T) {
	appJS, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"edit-form", "toggle-edit", "cancel-edit", "note-form", "dep-input", "link-input", "stale_revision", "data-nav-ticket"} {
		if !strings.Contains(string(appJS), required) {
			t.Fatalf("app.js missing editing marker %q", required)
		}
	}
}

func TestEmbeddedAppIncludesWorkbenchLayoutShell(t *testing.T) {
	index, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	appJS, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(index) + "\n" + string(appJS)
	for _, required := range []string{"panel-scroll", "detail-back", "view-detail", "view-list", "setMobileView"} {
		if !strings.Contains(combined, required) {
			t.Fatalf("embedded app missing workbench layout marker %q", required)
		}
	}
}

func TestEmbeddedAppIncludesHealthPanel(t *testing.T) {
	index, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	appJS, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(index) + "\n" + string(appJS)
	for _, required := range []string{"health-column", "nav-health", "refresh-health", "/api/health", "health-check", "setActiveView", "applyPaneSizes"} {
		if !strings.Contains(combined, required) {
			t.Fatalf("embedded app missing health marker %q", required)
		}
	}
}

func TestEmbeddedAppIncludesDiscoveryControls(t *testing.T) {
	index, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	appJS, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(index) + "\n" + string(appJS)
	for _, required := range []string{"ticket-inbox", "sort-pill", "data-sort-field", "open-sidebar", "toggle-sidebar", "sidebar-collapsed", "sidebar-drawer-open", "resize-handle", "type-epic", "type-mark-epic", "filter-chip", "tkt-web-selected-project", "tkt-web-sidebar-collapsed", "tkt-web-pane-widths"} {
		if !strings.Contains(combined, required) {
			t.Fatalf("embedded app missing discovery marker %q", required)
		}
	}
}

func TestEmbeddedAppIncludesMarkdownRendering(t *testing.T) {
	index, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	appJS, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	markdownJS, err := assets.ReadFile("assets/markdown.js")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(index) + "\n" + string(appJS) + "\n" + string(markdownJS)
	for _, required := range []string{
		"/assets/markdown.js",
		"tktMarkdown",
		"renderMarkdown",
		"markdown-body",
		"sanitizeHTML",
		"sectionHeading",
		"notesContent",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("embedded app missing markdown marker %q", required)
		}
	}
}

func TestHealthEndpointReturnsDoctorReport(t *testing.T) {
	_, repo, _ := setupWebProject(t)
	server, err := New(Options{Token: "secret", CWD: repo})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected ok, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Status string `json:"status"`
		Checks []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
			Status   string `json:"status"`
			Message  string `json:"message"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status == "" || len(payload.Checks) == 0 {
		t.Fatalf("expected doctor report, got %#v", payload)
	}
}

func TestHealthEndpointRejectsMutation(t *testing.T) {
	_, repo, _ := setupWebProject(t)
	server, err := New(Options{Token: "secret", CWD: repo})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/health", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateTicketViaAPI(t *testing.T) {
	home, repo, ticketDir := setupWebProject(t)
	server, err := New(Options{Token: "secret", CWD: repo})
	if err != nil {
		t.Fatal(err)
	}

	body := bytes.NewReader([]byte(`{"source":"test","id":"c-created","title":"Created from web","type":"feature","priority":1}`))
	req := httptest.NewRequest(http.MethodPost, "/api/projects/demo/tickets", body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected created, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(ticketDir, "c-created.md")); err != nil {
		t.Fatalf("expected ticket file: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".tkt", "state", "demo", "mutations.jsonl"))
	if err != nil {
		t.Fatalf("read mutation log: %v", err)
	}
	if !strings.Contains(string(raw), `"operation":"create"`) || !strings.Contains(string(raw), `"source":"test"`) {
		t.Fatalf("mutation log missing create/source: %s", string(raw))
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

type fakeListener struct {
	addr net.Addr
}

func (f fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (f fakeListener) Close() error              { return nil }
func (f fakeListener) Addr() net.Addr            { return f.addr }
