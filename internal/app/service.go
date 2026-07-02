package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrTicketNotFound  = errors.New("ticket not found")
	ErrStaleRevision   = errors.New("stale ticket revision")
	ErrValidation      = errors.New("validation error")
)

var (
	allowedStatusValues = []string{"open", "in_progress", "needs_testing", "closed"}
	allowedTypeValues   = []string{"bug", "feature", "task", "epic", "chore"}
)

type Service struct {
	cwd             string
	projectOverride string
	now             func() time.Time
}

type Options struct {
	CWD             string
	ProjectOverride string
	Now             func() time.Time
}

type ProjectOverview struct {
	Projects         []ProjectInfo `json:"projects"`
	ResolvedProject  string        `json:"resolved_project"`
	ResolutionSource string        `json:"resolution_source"`
	Initialized      bool          `json:"initialized"`
	Message          string        `json:"message,omitempty"`
}

type ProjectInfo struct {
	Name             string `json:"name"`
	Path             string `json:"-"`
	PathDisplay      string `json:"path"`
	Store            string `json:"store"`
	TicketDir        string `json:"-"`
	TicketDirDisplay string `json:"ticket_dir"`
	Current          bool   `json:"current"`
	Exists           bool   `json:"exists"`
	Writable         bool   `json:"writable"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	TicketID string `json:"ticket_id,omitempty"`
}

type Revision struct {
	ModTime string `json:"mod_time"`
	Hash    string `json:"hash"`
}

type TicketSummary struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Type     string   `json:"type"`
	Priority int      `json:"priority"`
	Assignee string   `json:"assignee"`
	Parent   string   `json:"parent"`
	Tags     []string `json:"tags"`
	Deps     []string `json:"deps"`
	Created  string   `json:"created"`
	Modified string   `json:"modified"`
	Revision Revision `json:"revision"`
}

type TicketRef struct {
	ID       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Status   string `json:"status,omitempty"`
	Type     string `json:"type,omitempty"`
	Priority int    `json:"priority,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
}

type TicketDetail struct {
	ID                 string                      `json:"id"`
	Title              string                      `json:"title"`
	Status             string                      `json:"status"`
	Type               string                      `json:"type"`
	Priority           int                         `json:"priority"`
	Assignee           string                      `json:"assignee"`
	Parent             string                      `json:"parent"`
	Tags               []string                    `json:"tags"`
	Created            string                      `json:"created"`
	ExternalRef        string                      `json:"external_ref"`
	Description        string                      `json:"description"`
	Design             string                      `json:"design"`
	AcceptanceCriteria string                      `json:"acceptance_criteria"`
	OtherSections      []ticket.Section            `json:"other_sections"`
	Deps               []TicketRef                 `json:"deps"`
	Links              []TicketRef                 `json:"links"`
	Children           []TicketSummary             `json:"children"`
	RecentCommits      []engine.CommitJournalEntry `json:"recent_commits"`
	Revision           Revision                    `json:"revision"`
	Diagnostics        []Diagnostic                `json:"diagnostics,omitempty"`
}

type ListOptions struct {
	Status   string
	Type     string
	Priority *int
	Assignee string
	Tag      string
	Parent   string
	Search   string
	Sort     string
	Limit    int
	OnlyOpen bool
	Ready    bool
	Blocked  bool
}

type TicketList struct {
	Items       []TicketSummary `json:"items"`
	Total       int             `json:"total"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

type CreateTicketInput struct {
	Source             string
	ID                 string
	Title              string
	Description        string
	Design             string
	AcceptanceCriteria string
	Type               string
	Priority           *int
	Assignee           string
	Parent             string
	Tags               []string
	ExternalRef        string
}

type UpdateTicketInput struct {
	Source             string
	ExpectedRevision   *Revision
	Title              *string
	Status             *string
	Type               *string
	Priority           *int
	Assignee           *string
	Parent             *string
	Tags               *[]string
	Description        *string
	Design             *string
	AcceptanceCriteria *string
	ExternalRef        *string
}

type NoteInput struct {
	Source           string
	Text             string
	ExpectedRevision *Revision
}

type EdgeInput struct {
	Source           string
	TargetID         string
	TargetIDs        []string
	ExpectedRevision *Revision
}

func New(options Options) *Service {
	cwd := options.CWD
	if strings.TrimSpace(cwd) == "" {
		if current, err := os.Getwd(); err == nil {
			cwd = current
		}
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		cwd:             cwd,
		projectOverride: options.ProjectOverride,
		now:             now,
	}
}

func (s *Service) Projects() (ProjectOverview, error) {
	cfg, err := project.Load()
	if err != nil {
		return ProjectOverview{}, err
	}
	resolved, source := project.ResolveName(cfg, s.cwd, s.projectOverride)
	_, initialized := cfg.Projects[resolved]

	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	projects := make([]ProjectInfo, 0, len(names))
	for _, name := range names {
		info, err := s.projectInfo(name, cfg.Projects[name], name == resolved)
		if err != nil {
			return ProjectOverview{}, err
		}
		projects = append(projects, info)
	}

	message := ""
	if !initialized {
		if resolved == "" {
			message = "No TKT project resolved for this directory."
		} else {
			message = fmt.Sprintf("Resolved %q from %s, but it is not registered in TKT config.", resolved, source)
		}
	}

	return ProjectOverview{
		Projects:         projects,
		ResolvedProject:  resolved,
		ResolutionSource: source,
		Initialized:      initialized,
		Message:          message,
	}, nil
}

func (s *Service) ListTickets(projectName string, options ListOptions) (TicketList, error) {
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketList{}, err
	}
	records, diagnostics, err := LoadTicketsWithDiagnostics(info.TicketDir)
	if err != nil {
		return TicketList{}, err
	}
	filtered := filterRecords(records, options)
	if options.Sort != "" {
		if err := engine.SortRecords(filtered, options.Sort); err != nil {
			return TicketList{}, err
		}
	} else {
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	}
	filtered = engine.LimitRecords(filtered, options.Limit)

	items := make([]TicketSummary, 0, len(filtered))
	for _, record := range filtered {
		summary, err := SummaryFromRecord(record)
		if err != nil {
			diagnostics = append(diagnostics, diagnosticForError(filepath.Base(record.Path), err))
			continue
		}
		items = append(items, summary)
	}
	return TicketList{Items: items, Total: len(items), Diagnostics: diagnostics}, nil
}

func (s *Service) TicketDetail(projectName, id string) (TicketDetail, error) {
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	records, diagnostics, err := LoadTicketsWithDiagnostics(info.TicketDir)
	if err != nil {
		return TicketDetail{}, err
	}
	record, ok := engine.ResolveRecordFromList(records, id)
	if !ok {
		return TicketDetail{}, ErrTicketNotFound
	}
	return s.detailFromRecords(projectName, record, records, diagnostics)
}

func (s *Service) CreateTicket(projectName string, input CreateTicketInput) (TicketDetail, error) {
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	if err := ticket.EnsureDir(info.TicketDir); err != nil {
		return TicketDetail{}, err
	}

	source, err := normalizeSource(input.Source)
	if err != nil {
		return TicketDetail{}, err
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		generated, err := ticket.GenerateID(info.TicketDir)
		if err != nil {
			return TicketDetail{}, err
		}
		id = generated
	} else if id, err = validateTicketIDField("ticket id", id, false); err != nil {
		return TicketDetail{}, err
	} else if _, err := os.Stat(filepath.Join(info.TicketDir, id+".md")); err == nil {
		return TicketDetail{}, validationErrorf("ticket id %q already exists", id)
	} else if !os.IsNotExist(err) {
		return TicketDetail{}, err
	}

	ticketType := strings.TrimSpace(input.Type)
	if ticketType == "" {
		ticketType = "task"
	}
	ticketType, err = validateEnumField("type", ticketType, allowedTypeValues)
	if err != nil {
		return TicketDetail{}, err
	}
	priority, err := normalizePriority(input.Priority)
	if err != nil {
		return TicketDetail{}, err
	}
	assignee, err := validateScalarField("assignee", input.Assignee, true)
	if err != nil {
		return TicketDetail{}, err
	}
	parent, err := validateTicketIDField("parent", input.Parent, true)
	if err != nil {
		return TicketDetail{}, err
	}
	tags, err := validateTagList(input.Tags)
	if err != nil {
		return TicketDetail{}, err
	}
	externalRef, err := validateScalarField("external_ref", input.ExternalRef, true)
	if err != nil {
		return TicketDetail{}, err
	}

	record := ticket.Record{
		ID:   id,
		Path: filepath.Join(info.TicketDir, id+".md"),
		Front: ticket.Frontmatter{
			ID:          id,
			Status:      "open",
			Deps:        []string{},
			Links:       []string{},
			Created:     s.now().UTC().Format(time.RFC3339),
			Type:        ticketType,
			Priority:    priority,
			Assignee:    assignee,
			Parent:      parent,
			Tags:        tags,
			ExternalRef: externalRef,
			Extra:       map[string]ticket.ExtraField{},
		},
		Body: ticket.Body{
			Title:              input.Title,
			Description:        input.Description,
			Design:             input.Design,
			AcceptanceCriteria: input.AcceptanceCriteria,
		},
	}
	if strings.TrimSpace(record.Body.Title) == "" {
		return TicketDetail{}, validationErrorf("title is required")
	}
	if err := ticket.SaveRecord(record); err != nil {
		return TicketDetail{}, err
	}
	s.appendMutation(projectName, engine.MutationEntry{
		TicketID:      id,
		Operation:     "create",
		Source:        source,
		FieldsChanged: []string{"title", "status", "type", "priority"},
	})
	return s.TicketDetail(projectName, id)
}

func (s *Service) UpdateTicket(projectName, id string, input UpdateTicketInput) (TicketDetail, error) {
	source, err := normalizeSource(input.Source)
	if err != nil {
		return TicketDetail{}, err
	}
	id, err = validateTicketIDField("ticket id", id, false)
	if err != nil {
		return TicketDetail{}, err
	}
	if input.Status != nil {
		value, err := validateEnumField("status", *input.Status, allowedStatusValues)
		if err != nil {
			return TicketDetail{}, err
		}
		input.Status = &value
	}
	if input.Type != nil {
		value, err := validateEnumField("type", *input.Type, allowedTypeValues)
		if err != nil {
			return TicketDetail{}, err
		}
		input.Type = &value
	}
	if input.Priority != nil {
		if err := validatePriority(*input.Priority); err != nil {
			return TicketDetail{}, err
		}
	}
	if input.Assignee != nil {
		value, err := validateScalarField("assignee", *input.Assignee, true)
		if err != nil {
			return TicketDetail{}, err
		}
		input.Assignee = &value
	}
	if input.Parent != nil {
		value, err := validateTicketIDField("parent", *input.Parent, true)
		if err != nil {
			return TicketDetail{}, err
		}
		input.Parent = &value
	}
	if input.Tags != nil {
		value, err := validateTagList(*input.Tags)
		if err != nil {
			return TicketDetail{}, err
		}
		input.Tags = &value
	}
	if input.ExternalRef != nil {
		value, err := validateScalarField("external_ref", *input.ExternalRef, true)
		if err != nil {
			return TicketDetail{}, err
		}
		input.ExternalRef = &value
	}

	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	record, err := ticket.LoadByID(info.TicketDir, id)
	if err != nil {
		return TicketDetail{}, normalizeTicketError(err)
	}
	if err := checkExpectedRevision(record.Path, input.ExpectedRevision); err != nil {
		return TicketDetail{}, err
	}

	changed := make([]string, 0)
	if input.Title != nil {
		record.Body.Title = *input.Title
		changed = append(changed, "title")
	}
	if input.Status != nil {
		record.Front.Status = *input.Status
		changed = append(changed, "status")
	}
	if input.Type != nil {
		record.Front.Type = *input.Type
		changed = append(changed, "type")
	}
	if input.Priority != nil {
		record.Front.Priority = *input.Priority
		changed = append(changed, "priority")
	}
	if input.Assignee != nil {
		record.Front.Assignee = *input.Assignee
		changed = append(changed, "assignee")
	}
	if input.Parent != nil {
		record.Front.Parent = *input.Parent
		changed = append(changed, "parent")
	}
	if input.Tags != nil {
		record.Front.Tags = *input.Tags
		changed = append(changed, "tags")
	}
	if input.Description != nil {
		record.Body.Description = *input.Description
		changed = append(changed, "description")
	}
	if input.Design != nil {
		record.Body.Design = *input.Design
		changed = append(changed, "design")
	}
	if input.AcceptanceCriteria != nil {
		record.Body.AcceptanceCriteria = *input.AcceptanceCriteria
		changed = append(changed, "acceptance_criteria")
	}
	if input.ExternalRef != nil {
		record.Front.ExternalRef = *input.ExternalRef
		changed = append(changed, "external_ref")
	}

	if err := ticket.SaveRecord(record); err != nil {
		return TicketDetail{}, err
	}
	s.appendMutation(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "edit",
		Source:        source,
		FieldsChanged: changed,
	})
	return s.TicketDetail(projectName, record.ID)
}

func (s *Service) AddNote(projectName, id string, input NoteInput) (TicketDetail, error) {
	source, err := normalizeSource(input.Source)
	if err != nil {
		return TicketDetail{}, err
	}
	id, err = validateTicketIDField("ticket id", id, false)
	if err != nil {
		return TicketDetail{}, err
	}
	if strings.TrimSpace(input.Text) == "" {
		return TicketDetail{}, validationErrorf("note text is required")
	}
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	record, err := ticket.LoadByID(info.TicketDir, id)
	if err != nil {
		return TicketDetail{}, normalizeTicketError(err)
	}
	if err := checkExpectedRevision(record.Path, input.ExpectedRevision); err != nil {
		return TicketDetail{}, err
	}

	ts := s.now().UTC().Format(time.RFC3339)
	header := fmt.Sprintf("**%s [%s]**", ts, source)
	entry := fmt.Sprintf("%s\n\n%s", header, strings.TrimSpace(input.Text))

	existing := engine.FindSection(record.Body.OtherSections, "Notes")
	if strings.TrimSpace(existing.Content) != "" {
		existing.Content = strings.TrimSpace(existing.Content) + "\n\n" + entry
	} else {
		existing.Content = entry
	}
	record.Body.OtherSections = engine.UpsertSection(record.Body.OtherSections, existing)

	if err := ticket.SaveRecord(record); err != nil {
		return TicketDetail{}, err
	}
	s.appendMutation(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "add-note",
		Source:        source,
		FieldsChanged: []string{"notes"},
	})
	return s.TicketDetail(projectName, record.ID)
}

func (s *Service) AddDependency(projectName, id string, input EdgeInput) (TicketDetail, error) {
	source, err := normalizeSource(input.Source)
	if err != nil {
		return TicketDetail{}, err
	}
	id, err = validateTicketIDField("ticket id", id, false)
	if err != nil {
		return TicketDetail{}, err
	}
	targetID, err := validateTicketIDField("target ticket id", input.TargetID, false)
	if err != nil {
		return TicketDetail{}, err
	}
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	record, err := ticket.LoadByID(info.TicketDir, id)
	if err != nil {
		return TicketDetail{}, normalizeTicketError(err)
	}
	if err := checkExpectedRevision(record.Path, input.ExpectedRevision); err != nil {
		return TicketDetail{}, err
	}
	depRecord, err := ticket.LoadByID(info.TicketDir, targetID)
	if err != nil {
		return TicketDetail{}, normalizeTicketError(err)
	}
	depID, err := validateTicketIDField("target ticket id", depRecord.ID, false)
	if err != nil {
		return TicketDetail{}, err
	}
	if record.ID == depRecord.ID {
		return TicketDetail{}, validationErrorf("ticket cannot depend on itself")
	}
	record.Front.Deps = engine.AppendUnique(record.Front.Deps, depID)
	if err := ticket.SaveRecord(record); err != nil {
		return TicketDetail{}, err
	}
	s.appendMutation(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "dep",
		Source:        source,
		FieldsChanged: []string{"deps"},
	})
	return s.TicketDetail(projectName, record.ID)
}

func (s *Service) RemoveDependency(projectName, id string, input EdgeInput) (TicketDetail, error) {
	source, err := normalizeSource(input.Source)
	if err != nil {
		return TicketDetail{}, err
	}
	id, err = validateTicketIDField("ticket id", id, false)
	if err != nil {
		return TicketDetail{}, err
	}
	targetID, err := validateTicketIDField("target ticket id", input.TargetID, false)
	if err != nil {
		return TicketDetail{}, err
	}
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	record, err := ticket.LoadByID(info.TicketDir, id)
	if err != nil {
		return TicketDetail{}, normalizeTicketError(err)
	}
	if err := checkExpectedRevision(record.Path, input.ExpectedRevision); err != nil {
		return TicketDetail{}, err
	}
	resolvedDepID := targetID
	if depRecord, err := ticket.LoadByID(info.TicketDir, targetID); err == nil {
		depID, err := validateTicketIDField("target ticket id", depRecord.ID, false)
		if err != nil {
			return TicketDetail{}, err
		}
		resolvedDepID = depID
	}
	record.Front.Deps = engine.RemoveValue(record.Front.Deps, resolvedDepID)
	if err := ticket.SaveRecord(record); err != nil {
		return TicketDetail{}, err
	}
	s.appendMutation(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "undep",
		Source:        source,
		FieldsChanged: []string{"deps"},
	})
	return s.TicketDetail(projectName, record.ID)
}

func (s *Service) LinkTickets(projectName, id string, input EdgeInput) (TicketDetail, error) {
	source, err := normalizeSource(input.Source)
	if err != nil {
		return TicketDetail{}, err
	}
	id, err = validateTicketIDField("ticket id", id, false)
	if err != nil {
		return TicketDetail{}, err
	}
	targetIDs := input.TargetIDs
	if len(targetIDs) == 0 && strings.TrimSpace(input.TargetID) != "" {
		targetIDs = []string{input.TargetID}
	}
	if len(targetIDs) == 0 {
		return TicketDetail{}, validationErrorf("target ticket id is required")
	}
	validatedTargetIDs := make([]string, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		targetID, err = validateTicketIDField("target ticket id", targetID, false)
		if err != nil {
			return TicketDetail{}, err
		}
		validatedTargetIDs = append(validatedTargetIDs, targetID)
	}
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	sourceRecord, err := ticket.LoadByID(info.TicketDir, id)
	if err != nil {
		return TicketDetail{}, normalizeTicketError(err)
	}
	if err := checkExpectedRevision(sourceRecord.Path, input.ExpectedRevision); err != nil {
		return TicketDetail{}, err
	}
	sourceRecordID, err := validateTicketIDField("source ticket id", sourceRecord.ID, false)
	if err != nil {
		return TicketDetail{}, err
	}

	targets := map[string]ticket.Record{}
	for _, targetID := range validatedTargetIDs {
		target, err := ticket.LoadByID(info.TicketDir, targetID)
		if err != nil {
			return TicketDetail{}, normalizeTicketError(err)
		}
		targetRecordID, err := validateTicketIDField("target ticket id", target.ID, false)
		if err != nil {
			return TicketDetail{}, err
		}
		if targetRecordID == sourceRecordID {
			return TicketDetail{}, validationErrorf("ticket cannot link to itself")
		}
		targets[targetRecordID] = target
	}

	sortedIDs := make([]string, 0, len(targets))
	for id := range targets {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, targetID := range sortedIDs {
		sourceRecord.Front.Links = engine.AppendUnique(sourceRecord.Front.Links, targetID)
	}
	if err := ticket.SaveRecord(sourceRecord); err != nil {
		return TicketDetail{}, err
	}
	for _, targetID := range sortedIDs {
		target := targets[targetID]
		target.Front.Links = engine.AppendUnique(target.Front.Links, sourceRecordID)
		if err := ticket.SaveRecord(target); err != nil {
			return TicketDetail{}, err
		}
	}
	s.appendMutation(projectName, engine.MutationEntry{
		TicketID:      sourceRecordID,
		Operation:     "link",
		Source:        source,
		FieldsChanged: []string{"links"},
	})
	return s.TicketDetail(projectName, sourceRecordID)
}

func (s *Service) UnlinkTicket(projectName, id string, input EdgeInput) (TicketDetail, error) {
	source, err := normalizeSource(input.Source)
	if err != nil {
		return TicketDetail{}, err
	}
	id, err = validateTicketIDField("ticket id", id, false)
	if err != nil {
		return TicketDetail{}, err
	}
	targetID, err := validateTicketIDField("target ticket id", input.TargetID, false)
	if err != nil {
		return TicketDetail{}, err
	}
	info, err := s.ResolveProject(projectName)
	if err != nil {
		return TicketDetail{}, err
	}
	sourceRecord, err := ticket.LoadByID(info.TicketDir, id)
	if err != nil {
		return TicketDetail{}, normalizeTicketError(err)
	}
	if err := checkExpectedRevision(sourceRecord.Path, input.ExpectedRevision); err != nil {
		return TicketDetail{}, err
	}
	sourceRecordID, err := validateTicketIDField("source ticket id", sourceRecord.ID, false)
	if err != nil {
		return TicketDetail{}, err
	}
	resolvedTargetID := targetID
	targetRecord, targetErr := ticket.LoadByID(info.TicketDir, targetID)
	if targetErr == nil {
		targetRecordID, err := validateTicketIDField("target ticket id", targetRecord.ID, false)
		if err != nil {
			return TicketDetail{}, err
		}
		resolvedTargetID = targetRecordID
	}

	sourceRecord.Front.Links = engine.RemoveValue(sourceRecord.Front.Links, resolvedTargetID)
	if err := ticket.SaveRecord(sourceRecord); err != nil {
		return TicketDetail{}, err
	}
	if targetErr == nil {
		targetRecord.Front.Links = engine.RemoveValue(targetRecord.Front.Links, sourceRecordID)
		if err := ticket.SaveRecord(targetRecord); err != nil {
			return TicketDetail{}, err
		}
	}
	s.appendMutation(projectName, engine.MutationEntry{
		TicketID:      sourceRecordID,
		Operation:     "unlink",
		Source:        source,
		FieldsChanged: []string{"links"},
	})
	return s.TicketDetail(projectName, sourceRecordID)
}

func (s *Service) ResolveProject(projectName string) (ProjectInfo, error) {
	cfg, err := project.Load()
	if err != nil {
		return ProjectInfo{}, err
	}
	if strings.TrimSpace(projectName) == "" {
		resolved, _ := project.ResolveName(cfg, s.cwd, s.projectOverride)
		projectName = resolved
	}
	entry, ok := cfg.Projects[projectName]
	if !ok {
		return ProjectInfo{}, ErrProjectNotFound
	}
	return s.projectInfo(projectName, entry, true)
}

func (s *Service) projectInfo(name string, entry project.ProjectConfig, current bool) (ProjectInfo, error) {
	store := entry.Store
	if store == "" {
		store = "local"
	}
	ticketDir := ""
	var err error
	if store == "central" {
		ticketDir, err = engine.CentralProjectDir(name)
		if err != nil {
			return ProjectInfo{}, err
		}
	} else if strings.TrimSpace(entry.Path) != "" {
		ticketDir = filepath.Join(entry.Path, ticket.DefaultDir)
	} else {
		ticketDir = ticket.DefaultDir
	}

	exists := dirExists(ticketDir)
	return ProjectInfo{
		Name:             name,
		Path:             entry.Path,
		PathDisplay:      displayPath(entry.Path),
		Store:            store,
		TicketDir:        ticketDir,
		TicketDirDisplay: displayPath(ticketDir),
		Current:          current,
		Exists:           exists,
		Writable:         dirWritable(ticketDir),
	}, nil
}

func (s *Service) detailFromRecords(projectName string, record ticket.Record, records []ticket.Record, diagnostics []Diagnostic) (TicketDetail, error) {
	byID := engine.IndexByID(records)
	revision, err := RevisionForPath(record.Path)
	if err != nil {
		return TicketDetail{}, err
	}

	deps := make([]TicketRef, 0, len(record.Front.Deps))
	for _, id := range record.Front.Deps {
		deps = append(deps, refForID(id, byID))
	}
	links := make([]TicketRef, 0, len(record.Front.Links))
	for _, id := range record.Front.Links {
		links = append(links, refForID(id, byID))
	}
	children := make([]TicketSummary, 0)
	for _, candidate := range records {
		if candidate.Front.Parent != record.ID {
			continue
		}
		summary, err := SummaryFromRecord(candidate)
		if err == nil {
			children = append(children, summary)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })

	commits, _ := engine.ReadJournalEntries(projectName)
	recent := engine.LastNJournalEntries(engine.FilterJournalByTickets(commits, []string{record.ID}), 5)

	tags := record.Front.Tags
	if tags == nil {
		tags = []string{}
	}
	return TicketDetail{
		ID:                 record.ID,
		Title:              record.Body.Title,
		Status:             record.Front.Status,
		Type:               record.Front.Type,
		Priority:           record.Front.Priority,
		Assignee:           record.Front.Assignee,
		Parent:             record.Front.Parent,
		Tags:               tags,
		Created:            record.Front.Created,
		ExternalRef:        record.Front.ExternalRef,
		Description:        record.Body.Description,
		Design:             record.Body.Design,
		AcceptanceCriteria: record.Body.AcceptanceCriteria,
		OtherSections:      record.Body.OtherSections,
		Deps:               deps,
		Links:              links,
		Children:           children,
		RecentCommits:      recent,
		Revision:           revision,
		Diagnostics:        diagnostics,
	}, nil
}

func (s *Service) appendMutation(projectName string, entry engine.MutationEntry) {
	engine.AppendMutationLog(projectName, entry)
}

func LoadTicketsWithDiagnostics(dir string) ([]ticket.Record, []Diagnostic, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ticket.Record{}, []Diagnostic{{
				Code:    "ticket_dir_missing",
				Message: fmt.Sprintf("ticket directory does not exist: %s", displayPath(dir)),
			}}, nil
		}
		return nil, nil, err
	}

	paths := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)

	records := make([]ticket.Record, 0, len(paths))
	diagnostics := make([]Diagnostic, 0)
	for _, path := range paths {
		record, err := ticket.LoadRecord(path)
		if err != nil {
			diagnostics = append(diagnostics, diagnosticForError(filepath.Base(path), err))
			continue
		}
		records = append(records, record)
	}
	return records, diagnostics, nil
}

func SummaryFromRecord(record ticket.Record) (TicketSummary, error) {
	revision, err := RevisionForPath(record.Path)
	if err != nil {
		return TicketSummary{}, err
	}
	tags := record.Front.Tags
	if tags == nil {
		tags = []string{}
	}
	deps := record.Front.Deps
	if deps == nil {
		deps = []string{}
	}
	modified := ""
	if !record.ModTime.IsZero() {
		modified = record.ModTime.UTC().Format(time.RFC3339Nano)
	}
	return TicketSummary{
		ID:       record.ID,
		Title:    record.Body.Title,
		Status:   record.Front.Status,
		Type:     record.Front.Type,
		Priority: record.Front.Priority,
		Assignee: record.Front.Assignee,
		Parent:   record.Front.Parent,
		Tags:     tags,
		Deps:     deps,
		Created:  record.Front.Created,
		Modified: modified,
		Revision: revision,
	}, nil
}

func RevisionForPath(path string) (Revision, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Revision{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Revision{}, err
	}
	sum := sha256.Sum256(raw)
	return Revision{
		ModTime: info.ModTime().UTC().Format(time.RFC3339Nano),
		Hash:    hex.EncodeToString(sum[:]),
	}, nil
}

func checkExpectedRevision(path string, expected *Revision) error {
	if expected == nil {
		return nil
	}
	current, err := RevisionForPath(path)
	if err != nil {
		return err
	}
	if current.ModTime != expected.ModTime || current.Hash != expected.Hash {
		return ErrStaleRevision
	}
	return nil
}

func filterRecords(records []ticket.Record, filters ListOptions) []ticket.Record {
	byID := engine.IndexByID(records)
	out := make([]ticket.Record, 0, len(records))
	for _, record := range records {
		if filters.OnlyOpen && record.Front.Status != "open" {
			continue
		}
		if filters.Status != "" && record.Front.Status != filters.Status {
			continue
		}
		if filters.Type != "" && record.Front.Type != filters.Type {
			continue
		}
		if filters.Priority != nil && record.Front.Priority != *filters.Priority {
			continue
		}
		if filters.Assignee != "" && record.Front.Assignee != filters.Assignee {
			continue
		}
		if filters.Tag != "" && !engine.Contains(record.Front.Tags, filters.Tag) {
			continue
		}
		if filters.Parent != "" && record.Front.Parent != filters.Parent {
			continue
		}
		if filters.Ready && (record.Front.Status != "open" || engine.HasOpenDeps(record, byID)) {
			continue
		}
		if filters.Blocked && (record.Front.Status != "open" || !engine.HasOpenDeps(record, byID)) {
			continue
		}
		if filters.Search != "" {
			q := strings.ToLower(filters.Search)
			id := strings.ToLower(record.ID)
			title := strings.ToLower(record.Body.Title)
			if !strings.Contains(id, q) && !strings.Contains(title, q) {
				continue
			}
		}
		out = append(out, record)
	}
	return out
}

func refForID(id string, byID map[string]ticket.Record) TicketRef {
	record, ok := byID[id]
	if !ok {
		return TicketRef{ID: id, Missing: true}
	}
	return TicketRef{
		ID:       record.ID,
		Title:    record.Body.Title,
		Status:   record.Front.Status,
		Type:     record.Front.Type,
		Priority: record.Front.Priority,
	}
}

func diagnosticForError(file string, err error) Diagnostic {
	code := "ticket_read_error"
	if errors.Is(err, ticket.ErrMissingFrontmatter) {
		code = "missing_frontmatter"
	} else if errors.Is(err, ticket.ErrMalformedFrontmatter) {
		code = "malformed_frontmatter"
	}
	return Diagnostic{
		Code:    code,
		Message: err.Error(),
		File:    file,
	}
}

func validationErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

func normalizeSource(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "web"
	}
	return validateScalarField("source", value, false)
}

func normalizePriority(value *int) (int, error) {
	if value == nil {
		return 2, nil
	}
	if err := validatePriority(*value); err != nil {
		return 0, err
	}
	return *value, nil
}

func validatePriority(value int) error {
	if value < 0 || value > 4 {
		return validationErrorf("priority must be between 0 and 4")
	}
	return nil
}

func validateEnumField(name, value string, allowed []string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", validationErrorf("%s is required", name)
	}
	if containsControlChar(value) {
		return "", validationErrorf("%s contains control characters", name)
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", validationErrorf("%s must be one of %s", name, strings.Join(allowed, ", "))
}

func validateScalarField(name, value string, allowEmpty bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if allowEmpty {
			return "", nil
		}
		return "", validationErrorf("%s is required", name)
	}
	if containsControlChar(value) {
		return "", validationErrorf("%s contains control characters", name)
	}
	return value, nil
}

func validateTicketIDField(name, value string, allowEmpty bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if allowEmpty {
			return "", nil
		}
		return "", validationErrorf("%s is required", name)
	}
	if value == "." || value == ".." || containsControlChar(value) || strings.ContainsAny(value, " /\\,[]") {
		return "", validationErrorf("invalid %s %q", name, value)
	}
	return value, nil
}

func validateTagList(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		tag := strings.TrimSpace(value)
		if tag == "" {
			continue
		}
		if containsControlChar(tag) || strings.ContainsAny(tag, ",[]") {
			return nil, validationErrorf("invalid tag %q", tag)
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out, nil
}

func containsControlChar(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}) >= 0
}

func normalizeTicketError(err error) error {
	if errors.Is(err, ticket.ErrTicketNotFound) {
		return ErrTicketNotFound
	}
	if errors.Is(err, ticket.ErrAmbiguousID) {
		return validationErrorf("%s", err.Error())
	}
	return err
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func dirWritable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0222 != 0
}

func displayPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~" + string(filepath.Separator) + strings.TrimPrefix(path, prefix)
	}
	return path
}
