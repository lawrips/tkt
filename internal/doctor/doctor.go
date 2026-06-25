package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lawrips/tkt/internal/app"
	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Check struct {
	ID          string            `json:"id"`
	Category    string            `json:"category"`
	Status      Status            `json:"status"`
	Message     string            `json:"message"`
	Remediation string            `json:"remediation,omitempty"`
	Command     string            `json:"command,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

type Summary struct {
	Pass  int    `json:"pass"`
	Warn  int    `json:"warn"`
	Fail  int    `json:"fail"`
	Total int    `json:"total"`
	Worst Status `json:"worst"`
}

type ProjectSummary struct {
	Name             string `json:"name,omitempty"`
	ResolutionSource string `json:"resolution_source,omitempty"`
	Registered       bool   `json:"registered"`
	Store            string `json:"store,omitempty"`
	Path             string `json:"path,omitempty"`
	TicketDir        string `json:"ticket_dir,omitempty"`
	CentralRoot      string `json:"central_root,omitempty"`
}

type Report struct {
	GeneratedAt string         `json:"generated_at"`
	Status      Status         `json:"status"`
	Summary     Summary        `json:"summary"`
	Project     ProjectSummary `json:"project"`
	Checks      []Check        `json:"checks"`
}

type Options struct {
	CWD                        string
	ProjectOverride            string
	Now                        func() time.Time
	AgentInstructionCandidates []string
}

func Run(options Options) Report {
	r := runner{
		options: options,
		now:     options.Now,
	}
	if r.now == nil {
		r.now = func() time.Time { return time.Now().UTC() }
	}
	r.run()
	return r.report
}

type runner struct {
	options Options
	now     func() time.Time
	report  Report
	cfg     project.Config
	cwd     string
}

func (r *runner) run() {
	r.report.GeneratedAt = r.now().UTC().Format(time.RFC3339)
	r.cwd = strings.TrimSpace(r.options.CWD)
	if r.cwd == "" {
		if cwd, err := os.Getwd(); err == nil {
			r.cwd = cwd
		} else {
			r.add(Check{
				ID:          "project.cwd",
				Category:    "project",
				Status:      StatusFail,
				Message:     "Could not resolve the current working directory.",
				Remediation: err.Error(),
			})
		}
	}

	configOK := r.checkGlobal()
	entry, registered := r.checkProject(configOK)
	if registered {
		r.checkTickets(entry)
		r.checkSync(entry)
	}
	r.checkAgent()
	r.finalize()
}

func (r *runner) checkGlobal() bool {
	configPath, configPathErr := project.ConfigPath()
	if configPathErr != nil {
		r.add(Check{
			ID:          "global.config_path",
			Category:    "global",
			Status:      StatusFail,
			Message:     "Could not resolve the TKT config path.",
			Remediation: configPathErr.Error(),
		})
		return false
	}

	cfg, err := project.Load()
	if err != nil {
		r.add(Check{
			ID:          "global.config",
			Category:    "global",
			Status:      StatusFail,
			Message:     "Could not load TKT config.",
			Remediation: err.Error(),
		})
		return false
	}
	r.cfg = cfg

	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			r.add(Check{
				ID:          "global.config",
				Category:    "global",
				Status:      StatusWarn,
				Message:     "No TKT config file exists yet.",
				Remediation: "Initialize this repo or register a project before expecting ticket commands to resolve automatically.",
				Command:     "tkt init",
				Details:     map[string]string{"path": displayPath(configPath)},
			})
		} else {
			r.add(Check{
				ID:          "global.config",
				Category:    "global",
				Status:      StatusFail,
				Message:     "Could not inspect TKT config file.",
				Remediation: err.Error(),
				Details:     map[string]string{"path": displayPath(configPath)},
			})
		}
	} else {
		r.add(Check{
			ID:       "global.config",
			Category: "global",
			Status:   StatusPass,
			Message:  fmt.Sprintf("TKT config loaded from %s.", displayPath(configPath)),
			Details:  map[string]string{"path": displayPath(configPath)},
		})
	}

	stateDir, err := stateDir()
	if err != nil {
		r.add(Check{
			ID:          "global.state_dir",
			Category:    "global",
			Status:      StatusFail,
			Message:     "Could not resolve TKT state directory.",
			Remediation: err.Error(),
		})
	} else if info, err := os.Stat(stateDir); err != nil {
		if os.IsNotExist(err) {
			r.add(Check{
				ID:          "global.state_dir",
				Category:    "global",
				Status:      StatusWarn,
				Message:     "TKT state directory does not exist yet.",
				Remediation: "It will be created by commands that need journal, mutation, pid, or log state.",
				Details:     map[string]string{"path": displayPath(stateDir)},
			})
		} else {
			r.add(Check{
				ID:          "global.state_dir",
				Category:    "global",
				Status:      StatusFail,
				Message:     "Could not inspect TKT state directory.",
				Remediation: err.Error(),
				Details:     map[string]string{"path": displayPath(stateDir)},
			})
		}
	} else if !info.IsDir() {
		r.add(Check{
			ID:          "global.state_dir",
			Category:    "global",
			Status:      StatusFail,
			Message:     "TKT state path exists but is not a directory.",
			Remediation: "Move or remove the path, then rerun the command that needs state.",
			Details:     map[string]string{"path": displayPath(stateDir)},
		})
	} else if !pathModeWritable(info) {
		r.add(Check{
			ID:          "global.state_dir",
			Category:    "global",
			Status:      StatusWarn,
			Message:     "TKT state directory may not be writable.",
			Remediation: "Check directory ownership and permissions.",
			Details:     map[string]string{"path": displayPath(stateDir)},
		})
	} else {
		r.add(Check{
			ID:       "global.state_dir",
			Category: "global",
			Status:   StatusPass,
			Message:  fmt.Sprintf("TKT state directory is present at %s.", displayPath(stateDir)),
			Details:  map[string]string{"path": displayPath(stateDir)},
		})
	}

	workflow, err := project.LoadWorkflow()
	if err != nil {
		r.add(Check{
			ID:          "global.workflow",
			Category:    "global",
			Status:      StatusFail,
			Message:     "Could not load workflow guidance.",
			Remediation: err.Error(),
		})
	} else if workflow.UsingDefault {
		r.add(Check{
			ID:          "global.workflow",
			Category:    "global",
			Status:      StatusWarn,
			Message:     "Using the embedded default workflow.",
			Remediation: "Create or customize the workflow file so agents and humans share local conventions.",
			Command:     "tkt workflow",
			Details:     map[string]string{"path": workflow.PathDisplay},
		})
	} else {
		r.add(Check{
			ID:       "global.workflow",
			Category: "global",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Workflow file is present at %s.", workflow.PathDisplay),
			Details:  map[string]string{"path": workflow.PathDisplay},
		})
	}

	return true
}

func (r *runner) checkProject(configOK bool) (project.ProjectConfig, bool) {
	if !configOK {
		r.add(Check{
			ID:          "project.resolution",
			Category:    "project",
			Status:      StatusFail,
			Message:     "Project resolution skipped because config could not be loaded.",
			Remediation: "Fix the TKT config error first.",
		})
		return project.ProjectConfig{}, false
	}

	name, source := project.ResolveName(r.cfg, r.cwd, r.options.ProjectOverride)
	r.report.Project.Name = name
	r.report.Project.ResolutionSource = source
	if strings.TrimSpace(name) == "" {
		r.add(Check{
			ID:          "project.resolution",
			Category:    "project",
			Status:      StatusFail,
			Message:     "No TKT project could be resolved for this directory.",
			Remediation: "Run setup from the repository root or pass --project explicitly.",
			Command:     "tkt init",
		})
		return project.ProjectConfig{}, false
	}

	entry, ok := r.cfg.Projects[name]
	if !ok {
		r.add(Check{
			ID:          "project.registration",
			Category:    "project",
			Status:      StatusFail,
			Message:     fmt.Sprintf("Resolved project %q from %s, but it is not registered.", name, source),
			Remediation: "Register this repository in TKT config.",
			Command:     "tkt init",
		})
		return project.ProjectConfig{}, false
	}
	r.report.Project.Registered = true

	store := entry.Store
	if store == "" {
		store = "local"
	}
	ticketDir, err := ticketDirForProject(name, entry)
	if err != nil {
		r.add(Check{
			ID:          "project.ticket_dir",
			Category:    "project",
			Status:      StatusFail,
			Message:     "Could not resolve ticket directory.",
			Remediation: err.Error(),
		})
		return entry, true
	}
	r.report.Project.Store = store
	r.report.Project.Path = displayPath(entry.Path)
	r.report.Project.TicketDir = displayPath(ticketDir)

	r.add(Check{
		ID:       "project.registration",
		Category: "project",
		Status:   StatusPass,
		Message:  fmt.Sprintf("Resolved registered project %q from %s.", name, source),
		Details:  map[string]string{"project": name, "source": source},
	})

	if strings.TrimSpace(entry.Path) == "" {
		r.add(Check{
			ID:          "project.path",
			Category:    "project",
			Status:      StatusWarn,
			Message:     "Registered project has no path recorded.",
			Remediation: "Re-run setup from the repository root if path-based resolution should work.",
			Command:     "tkt init",
		})
	} else if info, err := os.Stat(entry.Path); err != nil {
		r.add(Check{
			ID:          "project.path",
			Category:    "project",
			Status:      StatusFail,
			Message:     fmt.Sprintf("Registered project path is not accessible: %s.", displayPath(entry.Path)),
			Remediation: err.Error(),
			Details:     map[string]string{"path": displayPath(entry.Path)},
		})
	} else if !info.IsDir() {
		r.add(Check{
			ID:          "project.path",
			Category:    "project",
			Status:      StatusFail,
			Message:     fmt.Sprintf("Registered project path is not a directory: %s.", displayPath(entry.Path)),
			Remediation: "Update the project registration.",
			Command:     "tkt init",
			Details:     map[string]string{"path": displayPath(entry.Path)},
		})
	} else {
		r.add(Check{
			ID:       "project.path",
			Category: "project",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Registered project path exists at %s.", displayPath(entry.Path)),
			Details:  map[string]string{"path": displayPath(entry.Path)},
		})
	}

	if store != "local" && store != "central" {
		r.add(Check{
			ID:          "project.store",
			Category:    "project",
			Status:      StatusFail,
			Message:     fmt.Sprintf("Unknown ticket store mode %q.", store),
			Remediation: "Use local or central storage.",
			Command:     "tkt migrate --local",
		})
	} else {
		r.add(Check{
			ID:       "project.store",
			Category: "project",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Project uses %s ticket storage.", store),
			Details:  map[string]string{"store": store},
		})
	}

	return entry, true
}

func (r *runner) checkTickets(entry project.ProjectConfig) {
	ticketDir := r.report.Project.TicketDir
	if ticketDir == "" {
		return
	}
	rawTicketDir := expandDisplayPath(ticketDir)
	info, err := os.Stat(rawTicketDir)
	if err != nil {
		if os.IsNotExist(err) {
			r.add(Check{
				ID:          "project.ticket_dir",
				Category:    "project",
				Status:      StatusFail,
				Message:     fmt.Sprintf("Ticket directory is missing: %s.", ticketDir),
				Remediation: "Create a ticket or rerun setup for this project.",
				Command:     "tkt create \"First ticket\"",
				Details:     map[string]string{"path": ticketDir},
			})
			return
		}
		r.add(Check{
			ID:          "project.ticket_dir",
			Category:    "project",
			Status:      StatusFail,
			Message:     fmt.Sprintf("Could not inspect ticket directory: %s.", ticketDir),
			Remediation: err.Error(),
			Details:     map[string]string{"path": ticketDir},
		})
		return
	}
	if !info.IsDir() {
		r.add(Check{
			ID:          "project.ticket_dir",
			Category:    "project",
			Status:      StatusFail,
			Message:     fmt.Sprintf("Ticket path exists but is not a directory: %s.", ticketDir),
			Remediation: "Move the file out of the way or update project storage settings.",
			Details:     map[string]string{"path": ticketDir},
		})
		return
	}

	if !pathModeWritable(info) {
		r.add(Check{
			ID:          "project.ticket_dir",
			Category:    "project",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("Ticket directory may not be writable: %s.", ticketDir),
			Remediation: "Check directory ownership and permissions.",
			Details:     map[string]string{"path": ticketDir},
		})
	} else {
		r.add(Check{
			ID:       "project.ticket_dir",
			Category: "project",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Ticket directory is present at %s.", ticketDir),
			Details:  map[string]string{"path": ticketDir},
		})
	}

	records, diagnostics, err := app.LoadTicketsWithDiagnostics(rawTicketDir)
	if err != nil {
		r.add(Check{
			ID:          "project.ticket_parse",
			Category:    "project",
			Status:      StatusFail,
			Message:     "Could not read ticket directory.",
			Remediation: err.Error(),
			Details:     map[string]string{"path": ticketDir},
		})
		return
	}
	if len(diagnostics) > 0 {
		r.add(Check{
			ID:          "project.ticket_parse",
			Category:    "project",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("%d ticket file(s) have parse diagnostics; %d ticket(s) loaded.", len(diagnostics), len(records)),
			Remediation: "Open the listed ticket files and fix frontmatter or markdown structure.",
			Details: map[string]string{
				"valid_tickets": strconv.Itoa(len(records)),
				"diagnostics":   strconv.Itoa(len(diagnostics)),
			},
		})
		return
	}
	r.add(Check{
		ID:       "project.ticket_parse",
		Category: "project",
		Status:   StatusPass,
		Message:  fmt.Sprintf("%d ticket file(s) parsed successfully.", len(records)),
		Details:  map[string]string{"valid_tickets": strconv.Itoa(len(records))},
	})

	_ = entry
}

func (r *runner) checkSync(entry project.ProjectConfig) {
	store := entry.Store
	if store == "" {
		store = "local"
	}
	needsServe := store == "central" || entry.AutoLink || entry.AutoClose
	if needsServe {
		r.checkServeProcess()
	}
	if store != "central" {
		if !needsServe {
			r.add(Check{
				ID:       "sync.watch",
				Category: "sync",
				Status:   StatusPass,
				Message:  "Background serve/watch is optional for this local-store project.",
			})
		}
		return
	}

	root, err := engine.CentralStoreRoot()
	if err != nil {
		r.add(Check{
			ID:          "sync.central_root",
			Category:    "sync",
			Status:      StatusFail,
			Message:     "Could not resolve central store root.",
			Remediation: err.Error(),
		})
		return
	}
	r.report.Project.CentralRoot = displayPath(root)

	rootInfo, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			r.add(Check{
				ID:          "sync.central_root",
				Category:    "sync",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("Central store root does not exist yet: %s.", displayPath(root)),
				Remediation: "Start serve/watch or create the directory before using central ticket storage.",
				Command:     "tkt serve start",
				Details:     map[string]string{"path": displayPath(root)},
			})
		} else {
			r.add(Check{
				ID:          "sync.central_root",
				Category:    "sync",
				Status:      StatusFail,
				Message:     "Could not inspect central store root.",
				Remediation: err.Error(),
				Details:     map[string]string{"path": displayPath(root)},
			})
		}
		return
	}
	if !rootInfo.IsDir() {
		r.add(Check{
			ID:          "sync.central_root",
			Category:    "sync",
			Status:      StatusFail,
			Message:     fmt.Sprintf("Central store root is not a directory: %s.", displayPath(root)),
			Remediation: "Move the file out of the way or change TKT_ROOT.",
			Details:     map[string]string{"path": displayPath(root)},
		})
		return
	}
	r.add(Check{
		ID:       "sync.central_root",
		Category: "sync",
		Status:   StatusPass,
		Message:  fmt.Sprintf("Central store root exists at %s.", displayPath(root)),
		Details:  map[string]string{"path": displayPath(root)},
	})

	if !pathModeWritable(rootInfo) {
		r.add(Check{
			ID:          "sync.central_writable",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     "Central store root may not be writable.",
			Remediation: "Check central store ownership and permissions.",
			Details:     map[string]string{"path": displayPath(root)},
		})
	} else {
		r.add(Check{
			ID:       "sync.central_writable",
			Category: "sync",
			Status:   StatusPass,
			Message:  "Central store root appears writable from permissions.",
			Details:  map[string]string{"path": displayPath(root)},
		})
	}

	gitDir := filepath.Join(root, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		r.add(Check{
			ID:          "sync.central_git",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     "Central store root is not initialized as a git repository.",
			Remediation: "Initialize the central store git repo before expecting remote sync.",
			Command:     "git -C " + shellQuote(root) + " init",
			Details:     map[string]string{"path": displayPath(gitDir)},
		})
		return
	}
	r.add(Check{
		ID:       "sync.central_git",
		Category: "sync",
		Status:   StatusPass,
		Message:  "Central store root is a git repository.",
		Details:  map[string]string{"path": displayPath(gitDir)},
	})

	r.checkCentralGit(root)
}

func (r *runner) checkCentralGit(root string) {
	if blocked := readCentralSyncBlocked(root); blocked != "" {
		r.add(Check{
			ID:          "sync.central_blocked",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     "Central store sync has a persisted blocked marker.",
			Remediation: blocked,
			Details:     map[string]string{"path": displayPath(filepath.Join(root, ".git", "tkt-central-sync-blocked"))},
		})
	} else {
		r.add(Check{
			ID:       "sync.central_blocked",
			Category: "sync",
			Status:   StatusPass,
			Message:  "No central sync blocked marker is present.",
		})
	}

	remoteOut, err := git(root, "remote")
	if err != nil {
		r.add(Check{
			ID:          "sync.central_remote",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     "Could not inspect central store remotes.",
			Remediation: "Check central store git configuration.",
			Command:     "git -C " + shellQuote(root) + " remote",
		})
	} else {
		remotes := strings.Fields(remoteOut)
		if len(remotes) == 0 {
			r.add(Check{
				ID:          "sync.central_remote",
				Category:    "sync",
				Status:      StatusWarn,
				Message:     "Central store git repository has no remote configured.",
				Remediation: "Add the intended remote explicitly if this machine should publish central tickets.",
				Command:     "git -C " + shellQuote(root) + " remote add origin <url>",
			})
		} else {
			sort.Strings(remotes)
			r.add(Check{
				ID:       "sync.central_remote",
				Category: "sync",
				Status:   StatusPass,
				Message:  fmt.Sprintf("Central store has %d remote(s) configured.", len(remotes)),
				Details:  map[string]string{"remotes": strings.Join(remotes, ",")},
			})
		}
	}

	statusOut, err := git(root, "status", "--porcelain")
	if err != nil {
		r.add(Check{
			ID:          "sync.central_dirty",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     "Could not inspect central store working tree status.",
			Remediation: "Run git status in the central store to inspect it manually.",
			Command:     "git -C " + shellQuote(root) + " status --short",
		})
	} else if strings.TrimSpace(statusOut) != "" {
		r.add(Check{
			ID:          "sync.central_dirty",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("Central store has %d uncommitted change(s).", len(nonEmptyLines(statusOut))),
			Remediation: "Let serve/watch commit and sync ticket changes, or inspect the central store manually.",
			Command:     "tkt serve start",
		})
	} else {
		r.add(Check{
			ID:       "sync.central_dirty",
			Category: "sync",
			Status:   StatusPass,
			Message:  "Central store working tree is clean.",
		})
	}

	aheadOut, err := git(root, "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		r.add(Check{
			ID:          "sync.central_unpushed",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     "Could not determine central store upstream/ahead status.",
			Remediation: "Set an upstream branch if this machine should publish central ticket commits.",
			Command:     "git -C " + shellQuote(root) + " status -sb",
		})
		return
	}
	count := strings.TrimSpace(aheadOut)
	if count != "" && count != "0" {
		r.add(Check{
			ID:          "sync.central_unpushed",
			Category:    "sync",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("Central store has %s unpushed commit(s).", count),
			Remediation: "Let serve/watch push central store commits, or publish them manually.",
			Command:     "tkt serve start",
		})
		return
	}
	r.add(Check{
		ID:       "sync.central_unpushed",
		Category: "sync",
		Status:   StatusPass,
		Message:  "Central store has no unpushed commits relative to upstream.",
	})
}

func (r *runner) checkServeProcess() {
	pidPath, err := servePIDPath()
	if err != nil {
		r.add(Check{
			ID:          "sync.watch_process",
			Category:    "sync",
			Status:      StatusFail,
			Message:     "Could not resolve serve/watch pid path.",
			Remediation: err.Error(),
		})
		return
	}
	pid, running := runningPID(pidPath)
	if running {
		r.add(Check{
			ID:       "sync.watch_process",
			Category: "sync",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Background serve/watch appears to be running with pid %d.", pid),
			Details:  map[string]string{"pid_file": displayPath(pidPath), "pid": strconv.Itoa(pid)},
		})
		return
	}
	r.add(Check{
		ID:          "sync.watch_process",
		Category:    "sync",
		Status:      StatusWarn,
		Message:     "Background serve/watch does not appear to be running.",
		Remediation: "Start it if you want commit journaling, auto-close, MCP stdio hosting, or central store sync.",
		Command:     "tkt serve start",
		Details:     map[string]string{"pid_file": displayPath(pidPath)},
	})
}

func (r *runner) checkAgent() {
	candidates := r.options.AgentInstructionCandidates
	if len(candidates) == 0 {
		candidates = defaultAgentInstructionCandidates(r.cwd)
	}

	found := make([]string, 0)
	withTKT := make([]string, 0)
	for _, path := range uniqueStrings(candidates) {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		found = append(found, displayPath(path))
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.ToLower(string(raw))
		if strings.Contains(text, "tkt") && (strings.Contains(text, "workflow") || strings.Contains(text, "ticket")) {
			withTKT = append(withTKT, displayPath(path))
		}
	}

	if len(withTKT) > 0 {
		r.add(Check{
			ID:       "agent.instructions",
			Category: "agent",
			Status:   StatusPass,
			Message:  fmt.Sprintf("Found TKT guidance in %d agent instruction file(s).", len(withTKT)),
			Details:  map[string]string{"files": strings.Join(withTKT, ",")},
		})
		return
	}
	if len(found) > 0 {
		r.add(Check{
			ID:          "agent.instructions",
			Category:    "agent",
			Status:      StatusWarn,
			Message:     "Agent instruction files exist, but no TKT guidance was detected.",
			Remediation: "Add the TKT workflow guidance to the agent instruction file you use.",
			Command:     "tkt agent-instructions",
			Details:     map[string]string{"files": strings.Join(found, ",")},
		})
		return
	}
	r.add(Check{
		ID:          "agent.instructions",
		Category:    "agent",
		Status:      StatusWarn,
		Message:     "No known agent instruction file was found.",
		Remediation: "Add TKT guidance to AGENTS.md, CLAUDE.md, Cursor rules, or your agent's equivalent setup file.",
		Command:     "tkt agent-instructions",
	})
}

func (r *runner) add(check Check) {
	r.report.Checks = append(r.report.Checks, check)
}

func (r *runner) finalize() {
	summary := Summary{Worst: StatusPass}
	for _, check := range r.report.Checks {
		summary.Total++
		switch check.Status {
		case StatusFail:
			summary.Fail++
			summary.Worst = StatusFail
		case StatusWarn:
			summary.Warn++
			if summary.Worst != StatusFail {
				summary.Worst = StatusWarn
			}
		default:
			summary.Pass++
		}
	}
	r.report.Summary = summary
	r.report.Status = summary.Worst
}

func ticketDirForProject(projectName string, entry project.ProjectConfig) (string, error) {
	if entry.Store == "central" {
		return engine.CentralProjectDir(projectName)
	}
	if strings.TrimSpace(entry.Path) != "" {
		return filepath.Join(entry.Path, ticket.DefaultDir), nil
	}
	return ticket.DefaultDir, nil
}

func stateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tkt", "state"), nil
}

func servePIDPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "serve.pid"), nil
}

func runningPID(pidPath string) (int, bool) {
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return 0, false
	}
	return pid, true
}

func git(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func readCentralSyncBlocked(storeRoot string) string {
	raw, err := os.ReadFile(filepath.Join(storeRoot, ".git", "tkt-central-sync-blocked"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func defaultAgentInstructionCandidates(cwd string) []string {
	out := []string{}
	if cwd != "" {
		out = append(out,
			filepath.Join(cwd, "AGENTS.md"),
			filepath.Join(cwd, "CLAUDE.md"),
			filepath.Join(cwd, ".claude", "CLAUDE.md"),
			filepath.Join(cwd, ".codex", "AGENTS.md"),
			filepath.Join(cwd, ".cursor", "rules", "tkt.md"),
			filepath.Join(cwd, ".cursor", "rules", "tkt.mdc"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out,
			filepath.Join(home, ".claude", "CLAUDE.md"),
			filepath.Join(home, ".codex", "AGENTS.md"),
		)
	}
	return out
}

func pathModeWritable(info os.FileInfo) bool {
	return info.Mode().Perm()&0222 != 0
}

func nonEmptyLines(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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

func expandDisplayPath(path string) string {
	if strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"+string(filepath.Separator)))
		}
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	return path
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '-' || r == '_' || r == ':' || r == '+' || r == '=' ||
			(r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
