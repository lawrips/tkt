package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/lawrips/tkt/internal/app"
)

type Options struct {
	Token           string
	CWD             string
	ProjectOverride string
	Version         string
}

type Server struct {
	token           string
	cwd             string
	projectOverride string
	version         string
}

type apiError struct {
	Error apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func New(options Options) (*Server, error) {
	token := options.Token
	if strings.TrimSpace(token) == "" {
		generated, err := GenerateToken()
		if err != nil {
			return nil, err
		}
		token = generated
	}
	version := options.Version
	if version == "" {
		version = "dev"
	}
	return &Server{
		token:           token,
		cwd:             options.CWD,
		projectOverride: options.ProjectOverride,
		version:         version,
	}, nil
}

func GenerateToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *Server) Token() string {
	return s.token
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.serveAPI(w, r)
		return
	}
	s.serveIndex(w, r)
}

func (s *Server) URLFor(listener net.Listener) string {
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil || port == "" {
		host = "127.0.0.1"
		port = listener.Addr().String()
	}
	if host == "" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%s/?token=%s", host, port, s.token)
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TKT Web</title>
  <style>
    body { font-family: system-ui, sans-serif; margin: 2rem; color: #17202a; background: #f7f8fa; }
    main { max-width: 760px; margin: 0 auto; background: white; border: 1px solid #d9dee7; padding: 1.5rem; }
    code { background: #eef1f5; padding: 0.1rem 0.3rem; }
  </style>
</head>
<body>
  <main>
    <h1>TKT Web</h1>
    <p>The local web control plane is running.</p>
    <p>API health: <code>/api/session</code></p>
  </main>
</body>
</html>`))
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing or invalid web token.", nil)
		return
	}
	if r.URL.Path != "/api/session" {
		writeError(w, http.StatusNotFound, "not_found", "API route not found.", nil)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}

	svc := app.New(app.Options{CWD: s.cwd, ProjectOverride: s.projectOverride})
	overview, err := svc.Projects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  s.version,
		"projects": overview,
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return false
	}
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token == s.token
	}
	if token := strings.TrimSpace(r.Header.Get("X-TKT-Token")); token != "" {
		return token == s.token
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):]) == s.token
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	writeJSON(w, status, apiError{Error: apiErrorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

func Listen(addr string) (net.Listener, error) {
	if strings.TrimSpace(addr) == "" {
		addr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if tcp, ok := listener.Addr().(*net.TCPAddr); ok && !tcp.IP.IsLoopback() {
		_ = listener.Close()
		return nil, errors.New("tkt web only binds to loopback addresses by default")
	}
	return listener, nil
}
