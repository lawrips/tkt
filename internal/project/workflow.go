package project

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
)

const workflowFileName = "workflow.md"

//go:embed workflow.md
var defaultWorkflowContent string

type Workflow struct {
	Content      string
	Path         string
	PathDisplay  string
	UsingDefault bool
}

// WorkflowPath returns ~/.tkt/workflow.md.
func WorkflowPath() (string, error) {
	dir, err := configDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workflowFileName), nil
}

// WorkflowDisplayPath returns the user-facing workflow path, typically ~/.tkt/workflow.md.
func WorkflowDisplayPath() (string, error) {
	path, err := WorkflowPath()
	if err != nil {
		return "", err
	}
	return displayPath(path), nil
}

// LoadWorkflow reads ~/.tkt/workflow.md and falls back to the embedded default when missing.
func LoadWorkflow() (Workflow, error) {
	path, err := WorkflowPath()
	if err != nil {
		return Workflow{}, err
	}

	content, err := os.ReadFile(path)
	if err == nil {
		return Workflow{
			Content:      string(content),
			Path:         path,
			PathDisplay:  displayPath(path),
			UsingDefault: false,
		}, nil
	}
	if !os.IsNotExist(err) {
		return Workflow{}, err
	}

	return Workflow{
		Content:      defaultWorkflowContent,
		Path:         path,
		PathDisplay:  displayPath(path),
		UsingDefault: true,
	}, nil
}

// EnsureWorkflowFile writes ~/.tkt/workflow.md if it does not already exist.
func EnsureWorkflowFile() error {
	path, err := WorkflowPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultWorkflowContent), 0644)
}

func displayPath(path string) string {
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
