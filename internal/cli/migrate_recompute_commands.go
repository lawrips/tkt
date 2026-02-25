package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
)

func runMigrate(ctx context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	toCentral := false
	toLocal := false
	yes := false
	fs.BoolVar(&toCentral, "central", false, "")
	fs.BoolVar(&toLocal, "local", false, "")
	fs.BoolVar(&yes, "yes", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: tkt migrate --central|--local [--yes]")
	}
	if toCentral == toLocal {
		return fmt.Errorf("must specify exactly one of --central or --local")
	}

	cfg, projectName, entry, err := resolveCurrentProject(ctx)
	if err != nil {
		return err
	}

	localDir := filepath.Join(entry.Path, ".tickets")
	centralDir, err := centralProjectDir(projectName)
	if err != nil {
		return err
	}

	source := localDir
	destination := centralDir
	targetStore := "central"
	if toLocal {
		source = centralDir
		destination = localDir
		targetStore = "local"
	}

	files, err := ticketFiles(source)
	if err != nil {
		return err
	}

	if ctx.json {
		plan := map[string]any{
			"project":      projectName,
			"source":       source,
			"destination":  destination,
			"files":        files,
			"target_store": targetStore,
		}
		if !yes {
			plan["confirmation_required"] = true
		}
		if err := emitJSON(ctx, plan); err != nil {
			return err
		}
	}

	if !ctx.json {
		_, _ = fmt.Fprintln(ctx.stdout, "Migration plan:")
		_, _ = fmt.Fprintf(ctx.stdout, "  project: %s\n", projectName)
		_, _ = fmt.Fprintf(ctx.stdout, "  source: %s\n", source)
		_, _ = fmt.Fprintf(ctx.stdout, "  destination: %s\n", destination)
		if len(files) == 0 {
			_, _ = fmt.Fprintln(ctx.stdout, "  files: (none)")
		} else {
			_, _ = fmt.Fprintf(ctx.stdout, "  files (%d): %s\n", len(files), strings.Join(files, ", "))
		}
	}

	if !yes {
		ok, err := promptConfirm(ctx, "Proceed with move? [y/N]: ")
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("migration cancelled")
		}
	}

	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	moved, err := moveFilesByName(source, destination, files)
	if err != nil {
		return err
	}
	_ = removeDirIfEmpty(source)

	entry.Store = targetStore
	cfg.UpsertProject(projectName, entry)
	if err := project.Save(cfg); err != nil {
		return err
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"project":      projectName,
			"moved":        moved,
			"source":       source,
			"destination":  destination,
			"target_store": targetStore,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "Moved %d ticket file(s) to %s\n", len(moved), destination)
	_, _ = fmt.Fprintf(ctx.stdout, "Updated project store: %s\n", targetStore)
	return nil
}

func runRecompute(ctx context, args []string) error {
	fs := flag.NewFlagSet("recompute", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	yes := false
	fs.BoolVar(&yes, "yes", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: tkt recompute [--yes]")
	}

	_, projectName, entry, err := resolveCurrentProject(ctx)
	if err != nil {
		return err
	}

	if !yes && !ctx.json {
		ok, err := promptConfirm(ctx, fmt.Sprintf("Rebuild commit journal for project %q from git log? [y/N]: ", projectName))
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("recompute cancelled")
		}
	}

	entries, err := buildJournalFromGitLog(entry.Path)
	if err != nil {
		return err
	}

	path, err := engine.JournalPath(projectName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return err
		}
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"project":       projectName,
			"journal_path":  path,
			"entries_count": len(entries),
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "Rebuilt %s with %d entries\n", path, len(entries))
	return nil
}

func resolveCurrentProject(ctx context) (project.Config, string, project.ProjectConfig, error) {
	cfg, err := project.Load()
	if err != nil {
		return project.Config{}, "", project.ProjectConfig{}, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return project.Config{}, "", project.ProjectConfig{}, err
	}
	name, _ := project.ResolveName(cfg, cwd, strings.TrimSpace(ctx.projectOverride))
	if name == "" {
		return project.Config{}, "", project.ProjectConfig{}, errors.New("no project resolved; run `tkt init` first or pass --project")
	}

	entry, ok := cfg.Projects[name]
	if !ok {
		return project.Config{}, "", project.ProjectConfig{}, fmt.Errorf("project %q not found in config", name)
	}
	return cfg, name, entry, nil
}

func ticketFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func moveFilesByName(sourceDir, destinationDir string, names []string) ([]string, error) {
	moved := make([]string, 0, len(names))
	for _, name := range names {
		sourcePath := filepath.Join(sourceDir, name)
		destPath := filepath.Join(destinationDir, name)

		raw, err := os.ReadFile(sourcePath)
		if err != nil {
			return moved, err
		}
		if err := os.WriteFile(destPath, raw, 0644); err != nil {
			return moved, err
		}
		if err := os.Remove(sourcePath); err != nil {
			return moved, err
		}
		moved = append(moved, name)
	}
	return moved, nil
}

func removeDirIfEmpty(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) == 0 {
		return os.Remove(path)
	}
	return nil
}

func promptConfirm(ctx context, prompt string) (bool, error) {
	_, _ = fmt.Fprint(ctx.stdout, prompt)
	reader := bufio.NewReader(ctx.stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	value := strings.ToLower(strings.TrimSpace(line))
	return value == "y" || value == "yes", nil
}

func buildJournalFromGitLog(repoPath string) ([]engine.CommitJournalEntry, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "--reverse", "--pretty=format:%H%x1f%cI%x1f%an%x1f%B%x1e")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	records := strings.Split(string(out), "\x1e")
	entries := make([]engine.CommitJournalEntry, 0)
	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\x1f", 4)
		if len(parts) < 4 {
			continue
		}

		sha := parts[0]
		ts := parts[1]
		author := parts[2]
		msg := strings.TrimSpace(parts[3])

		ticketActions := extractTicketActions(msg)
		if len(ticketActions) == 0 {
			continue
		}

		files, added, removed, branch, _ := getDiffStats(repoPath, sha)

		tickets := make([]string, 0, len(ticketActions))
		for ticketID := range ticketActions {
			tickets = append(tickets, ticketID)
		}
		sort.Strings(tickets)
		for _, ticketID := range tickets {
			entries = append(entries, engine.CommitJournalEntry{
				SHA:          sha,
				Ticket:       ticketID,
				Repo:         repoPath,
				TS:           ts,
				Msg:          msg,
				Author:       author,
				Action:       ticketActions[ticketID],
				FilesChanged: files,
				LinesAdded:   added,
				LinesRemoved: removed,
				Branch:       branch,
			})
		}
	}
	return entries, nil
}

// getDiffStats runs git diff-tree --numstat for a commit and returns the list of
// changed files, total lines added, total lines removed, and the branch name.
// Errors are silently ignored — callers get zero values on failure.
func getDiffStats(repoPath, sha string) (files []string, added, removed int, branch string, err error) {
	out, err := exec.Command("git", "-C", repoPath, "diff-tree", "--numstat", "--root", "-r", sha).Output()
	if err != nil {
		return nil, 0, 0, "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		r, _ := strconv.Atoi(parts[1])
		added += a
		removed += r
		if parts[2] != "" {
			files = append(files, parts[2])
		}
	}
	branch = resolveStableBranchName(repoPath, sha)

	return files, added, removed, branch, nil
}

// resolveStableBranchName returns a single stable branch label for a commit.
// If multiple local/remote branches contain the commit, it returns empty.
func resolveStableBranchName(repoPath, sha string) string {
	local := listContainingRefs(repoPath, sha, "refs/heads")
	if len(local) == 1 {
		return local[0]
	}
	if len(local) > 1 {
		return ""
	}

	remote := listContainingRefs(repoPath, sha, "refs/remotes")
	filtered := make([]string, 0, len(remote))
	for _, name := range remote {
		if strings.HasSuffix(name, "/HEAD") {
			continue
		}
		filtered = append(filtered, name)
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return ""
}

func listContainingRefs(repoPath, sha, refPrefix string) []string {
	out, err := exec.Command(
		"git", "-C", repoPath,
		"for-each-ref", "--format=%(refname:short)", "--contains", sha, refPrefix,
	).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	seen := map[string]struct{}{}
	refs := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		refs = append(refs, name)
	}
	sort.Strings(refs)
	return refs
}

// getParentCommitTS returns the author date of the parent commit (sha^),
// used as a work_started proxy. Returns empty string on any error.
func getParentCommitTS(repoPath, sha string) string {
	out, err := exec.Command("git", "-C", repoPath, "log", "-1", "--pretty=format:%aI", sha+"^").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
