package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/lawrips/tkt/internal/app"
	"github.com/lawrips/tkt/internal/doctor"
)

type Options struct {
	Token           string
	CWD             string
	ProjectOverride string
	Version         string
}

const DefaultAddr = "127.0.0.1:7420"

type Server struct {
	token           string
	cwd             string
	projectOverride string
	version         string
	addr            string
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

// SetAddr records the server's resolved listen address so origin checks can
// accept only the active server's own origin rather than any localhost port.
func (s *Server) SetAddr(addr string) {
	s.addr = addr
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.serveAPI(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		s.serveAsset(w, r)
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
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%s/?token=%s", host, port, s.token)
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	raw, err := assets.ReadFile("assets/index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(raw)
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
		return
	}
	http.StripPrefix("/assets/", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Missing or invalid web token.", nil)
		return
	}
	if isMutationMethod(r.Method) && !s.allowedOrigin(r) {
		writeError(w, http.StatusForbidden, "forbidden_origin", "Unexpected request origin.", nil)
		return
	}

	svc := app.New(app.Options{CWD: s.cwd, ProjectOverride: s.projectOverride})
	switch {
	case r.URL.Path == "/api/session":
		s.handleSession(w, r, svc)
	case r.URL.Path == "/api/health":
		s.handleHealth(w, r)
	case r.URL.Path == "/api/projects":
		s.handleProjects(w, r, svc)
	case strings.HasPrefix(r.URL.Path, "/api/projects/"):
		s.handleProjectRoute(w, r, svc)
	default:
		writeError(w, http.StatusNotFound, "not_found", "API route not found.", nil)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	report := doctor.Run(doctor.Options{
		CWD:             s.cwd,
		ProjectOverride: s.projectOverride,
	})
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request, svc *app.Service) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	overview, err := svc.Projects()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  s.version,
		"projects": overview,
	})
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request, svc *app.Service) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	overview, err := svc.Projects()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleProjectRoute(w http.ResponseWriter, r *http.Request, svc *app.Service) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		writeError(w, http.StatusNotFound, "not_found", "API route not found.", nil)
		return
	}
	projectName, ok := cleanPathPart(parts[0])
	if !ok {
		writeError(w, http.StatusBadRequest, "validation_error", "Invalid project name.", nil)
		return
	}
	if parts[1] == "insights" {
		s.handleInsights(w, r, svc, projectName, parts)
		return
	}
	if parts[1] != "tickets" {
		writeError(w, http.StatusNotFound, "not_found", "API route not found.", nil)
		return
	}

	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			s.handleTicketList(w, r, svc, projectName)
		case http.MethodPost:
			s.handleTicketCreate(w, r, svc, projectName)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		}
		return
	}

	ticketID, ok := cleanPathPart(parts[2])
	if !ok {
		writeError(w, http.StatusBadRequest, "validation_error", "Invalid ticket id.", nil)
		return
	}
	if len(parts) == 3 {
		switch r.Method {
		case http.MethodGet:
			detail, err := svc.TicketDetail(projectName, ticketID)
			if err != nil {
				writeAppError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, detail)
		case http.MethodPatch:
			s.handleTicketUpdate(w, r, svc, projectName, ticketID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		}
		return
	}

	switch parts[3] {
	case "notes":
		if len(parts) != 4 || r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
			return
		}
		s.handleAddNote(w, r, svc, projectName, ticketID)
	case "deps":
		s.handleDependencyRoute(w, r, svc, projectName, ticketID, parts)
	case "links":
		s.handleLinkRoute(w, r, svc, projectName, ticketID, parts)
	default:
		writeError(w, http.StatusNotFound, "not_found", "API route not found.", nil)
	}
}

func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request, svc *app.Service, projectName string, parts []string) {
	if len(parts) != 3 {
		writeError(w, http.StatusNotFound, "not_found", "API route not found.", nil)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
		return
	}
	switch parts[2] {
	case "stats":
		report, err := svc.Stats(projectName)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	case "timeline":
		weeks := 0
		if raw := r.URL.Query().Get("weeks"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				writeError(w, http.StatusBadRequest, "validation_error", fmt.Sprintf("invalid weeks %q", raw), nil)
				return
			}
			weeks = parsed
		}
		report, err := svc.Timeline(projectName, weeks)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	case "epics":
		report, err := svc.EpicOverview(projectName)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	default:
		writeError(w, http.StatusNotFound, "not_found", "API route not found.", nil)
	}
}

func (s *Server) handleTicketList(w http.ResponseWriter, r *http.Request, svc *app.Service, projectName string) {
	options, err := listOptionsFromQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	list, err := svc.ListTickets(projectName, options)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleTicketCreate(w http.ResponseWriter, r *http.Request, svc *app.Service, projectName string) {
	var req createTicketRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	detail, err := svc.CreateTicket(projectName, app.CreateTicketInput{
		Source:             firstNonEmpty(req.Source, "web"),
		ID:                 req.ID,
		Title:              req.Title,
		Description:        req.Description,
		Design:             req.Design,
		AcceptanceCriteria: req.AcceptanceCriteria,
		Type:               req.Type,
		Priority:           req.Priority,
		Assignee:           req.Assignee,
		Parent:             req.Parent,
		Tags:               req.Tags,
		ExternalRef:        req.ExternalRef,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (s *Server) handleTicketUpdate(w http.ResponseWriter, r *http.Request, svc *app.Service, projectName, ticketID string) {
	var req updateTicketRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	input := app.UpdateTicketInput{
		Source:             firstNonEmpty(req.Source, "web"),
		ExpectedRevision:   &req.Revision,
		Title:              req.Fields.Title,
		Status:             req.Fields.Status,
		Type:               req.Fields.Type,
		Priority:           req.Fields.Priority,
		Assignee:           req.Fields.Assignee,
		Parent:             req.Fields.Parent,
		Tags:               req.Fields.Tags,
		Description:        req.Fields.Description,
		Design:             req.Fields.Design,
		AcceptanceCriteria: req.Fields.AcceptanceCriteria,
		ExternalRef:        req.Fields.ExternalRef,
	}
	detail, err := svc.UpdateTicket(projectName, ticketID, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleAddNote(w http.ResponseWriter, r *http.Request, svc *app.Service, projectName, ticketID string) {
	var req noteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	detail, err := svc.AddNote(projectName, ticketID, app.NoteInput{
		Source:           firstNonEmpty(req.Source, "web"),
		Text:             req.Text,
		ExpectedRevision: &req.Revision,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleDependencyRoute(w http.ResponseWriter, r *http.Request, svc *app.Service, projectName, ticketID string, parts []string) {
	switch {
	case len(parts) == 4 && r.Method == http.MethodPost:
		var req edgeRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
			return
		}
		detail, err := svc.AddDependency(projectName, ticketID, app.EdgeInput{
			Source:           firstNonEmpty(req.Source, "web"),
			TargetID:         req.TargetID,
			ExpectedRevision: &req.Revision,
		})
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	case len(parts) == 5 && r.Method == http.MethodDelete:
		depID, ok := cleanPathPart(parts[4])
		if !ok {
			writeError(w, http.StatusBadRequest, "validation_error", "Invalid dependency id.", nil)
			return
		}
		revision, err := revisionFromQuery(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
			return
		}
		detail, err := svc.RemoveDependency(projectName, ticketID, app.EdgeInput{
			Source:           firstNonEmpty(r.URL.Query().Get("source"), "web"),
			TargetID:         depID,
			ExpectedRevision: revision,
		})
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) handleLinkRoute(w http.ResponseWriter, r *http.Request, svc *app.Service, projectName, ticketID string, parts []string) {
	switch {
	case len(parts) == 4 && r.Method == http.MethodPost:
		var req edgeRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
			return
		}
		targetIDs := req.TargetIDs
		if len(targetIDs) == 0 && req.TargetID != "" {
			targetIDs = []string{req.TargetID}
		}
		detail, err := svc.LinkTickets(projectName, ticketID, app.EdgeInput{
			Source:           firstNonEmpty(req.Source, "web"),
			TargetIDs:        targetIDs,
			ExpectedRevision: &req.Revision,
		})
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	case len(parts) == 5 && r.Method == http.MethodDelete:
		targetID, ok := cleanPathPart(parts[4])
		if !ok {
			writeError(w, http.StatusBadRequest, "validation_error", "Invalid target id.", nil)
			return
		}
		revision, err := revisionFromQuery(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
			return
		}
		detail, err := svc.UnlinkTicket(projectName, ticketID, app.EdgeInput{
			Source:           firstNonEmpty(r.URL.Query().Get("source"), "web"),
			TargetID:         targetID,
			ExpectedRevision: revision,
		})
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.", nil)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return false
	}
	if token := strings.TrimSpace(r.Header.Get("X-TKT-Token")); token != "" {
		return tokenEqual(token, s.token)
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return tokenEqual(strings.TrimSpace(auth[len("bearer "):]), s.token)
	}
	return false
}

// tokenEqual compares bearer tokens in constant time to avoid timing oracles.
func tokenEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) allowedOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return false
	}
	// When the server knows its own address, require the origin port to match
	// so a different local app on another localhost port cannot reach this API.
	if s.addr != "" {
		_, serverPort, err := net.SplitHostPort(s.addr)
		if err == nil && serverPort != "" {
			originPort := parsed.Port()
			if originPort == "" {
				return false
			}
			return originPort == serverPort
		}
	}
	return true
}

func isMutationMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

type createTicketRequest struct {
	Source             string   `json:"source"`
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Design             string   `json:"design"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Type               string   `json:"type"`
	Priority           *int     `json:"priority"`
	Assignee           string   `json:"assignee"`
	Parent             string   `json:"parent"`
	Tags               []string `json:"tags"`
	ExternalRef        string   `json:"external_ref"`
}

type updateTicketRequest struct {
	Revision app.Revision `json:"revision"`
	Source   string       `json:"source"`
	Fields   updateFields `json:"fields"`
}

type updateFields struct {
	Title              *string   `json:"title"`
	Status             *string   `json:"status"`
	Type               *string   `json:"type"`
	Priority           *int      `json:"priority"`
	Assignee           *string   `json:"assignee"`
	Parent             *string   `json:"parent"`
	Tags               *[]string `json:"tags"`
	Description        *string   `json:"description"`
	Design             *string   `json:"design"`
	AcceptanceCriteria *string   `json:"acceptance_criteria"`
	ExternalRef        *string   `json:"external_ref"`
}

type noteRequest struct {
	Revision app.Revision `json:"revision"`
	Source   string       `json:"source"`
	Text     string       `json:"text"`
}

type edgeRequest struct {
	Revision  app.Revision `json:"revision"`
	Source    string       `json:"source"`
	TargetID  string       `json:"target_id"`
	TargetIDs []string     `json:"target_ids"`
}

func listOptionsFromQuery(values url.Values) (app.ListOptions, error) {
	options := app.ListOptions{
		Status:   values.Get("status"),
		Type:     values.Get("type"),
		Assignee: values.Get("assignee"),
		Tag:      values.Get("tag"),
		Parent:   values.Get("parent"),
		Search:   values.Get("search"),
		Sort:     values.Get("sort"),
		OnlyOpen: values.Get("only_open") == "true",
		Ready:    values.Get("ready") == "true",
		Blocked:  values.Get("blocked") == "true",
	}
	if raw := values.Get("priority"); raw != "" {
		priority, err := strconv.Atoi(raw)
		if err != nil {
			return app.ListOptions{}, fmt.Errorf("invalid priority %q", raw)
		}
		options.Priority = &priority
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return app.ListOptions{}, fmt.Errorf("invalid limit %q", raw)
		}
		options.Limit = limit
	}
	return options, nil
}

func revisionFromQuery(values url.Values) (*app.Revision, error) {
	hash := values.Get("revision_hash")
	modTime := values.Get("revision_mod_time")
	if hash == "" && modTime == "" {
		return nil, nil
	}
	if hash == "" || modTime == "" {
		return nil, errors.New("revision_hash and revision_mod_time are both required")
	}
	return &app.Revision{Hash: hash, ModTime: modTime}, nil
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	return nil
}

func cleanPathPart(raw string) (string, bool) {
	value, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return "", false
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return "", false
	}
	return value, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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

func writeAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrProjectNotFound):
		writeError(w, http.StatusNotFound, "project_not_found", "Project not found.", nil)
	case errors.Is(err, app.ErrTicketNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Ticket not found.", nil)
	case errors.Is(err, app.ErrStaleRevision):
		writeError(w, http.StatusConflict, "stale_revision", "Ticket changed on disk. Refresh before saving.", nil)
	case errors.Is(err, app.ErrValidation):
		writeError(w, http.StatusBadRequest, "validation_error", validationMessage(err), nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error(), nil)
	}
}

func validationMessage(err error) string {
	message := err.Error()
	prefix := app.ErrValidation.Error() + ": "
	return strings.TrimPrefix(message, prefix)
}

func Listen(addr string) (net.Listener, error) {
	listener, err := net.Listen("tcp", NormalizeAddr(addr))
	if err != nil {
		return nil, err
	}
	if tcp, ok := listener.Addr().(*net.TCPAddr); ok && !tcp.IP.IsLoopback() {
		_ = listener.Close()
		return nil, errors.New("tkt web only binds to loopback addresses by default")
	}
	return listener, nil
}

func NormalizeAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return DefaultAddr
	}
	return addr
}
