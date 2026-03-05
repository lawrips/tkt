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

// GlobalWorkflowPath returns ~/.tkt/workflow.md.
func GlobalWorkflowPath() (string, error) {
	dir, err := configDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workflowFileName), nil
}

// WorkflowPath returns ~/.tkt/workflow.md.
// Deprecated: use GlobalWorkflowPath instead.
func WorkflowPath() (string, error) {
	return GlobalWorkflowPath()
}

// WorkflowDisplayPath returns the user-facing workflow path, typically ~/.tkt/workflow.md.
func WorkflowDisplayPath() (string, error) {
	path, err := GlobalWorkflowPath()
	if err != nil {
		return "", err
	}
	return displayPath(path), nil
}

// ProjectWorkflowPath returns <projectRoot>/.tkt/workflow.md.
func ProjectWorkflowPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".tkt", workflowFileName)
}

// LoadWorkflow checks for a project-level workflow first, then falls back to
// global ~/.tkt/workflow.md, then the embedded default.
//
// Precedence:
//  1. <projectRoot>/.tkt/workflow.md (if projectRoot is non-empty)
//  2. ~/.tkt/workflow.md
//  3. Embedded default
func LoadWorkflow(projectRoot string) (Workflow, error) {
	// 1. Project-level override.
	if projectRoot != "" {
		projPath := ProjectWorkflowPath(projectRoot)
		content, err := os.ReadFile(projPath)
		if err == nil {
			return Workflow{
				Content:      string(content),
				Path:         projPath,
				PathDisplay:  displayPath(projPath),
				UsingDefault: false,
			}, nil
		}
		if !os.IsNotExist(err) {
			return Workflow{}, err
		}
	}

	// 2. Global user config.
	globalPath, err := GlobalWorkflowPath()
	if err != nil {
		return Workflow{}, err
	}
	content, err := os.ReadFile(globalPath)
	if err == nil {
		return Workflow{
			Content:      string(content),
			Path:         globalPath,
			PathDisplay:  displayPath(globalPath),
			UsingDefault: false,
		}, nil
	}
	if !os.IsNotExist(err) {
		return Workflow{}, err
	}

	// 3. Embedded default.
	return Workflow{
		Content:      defaultWorkflowContent,
		Path:         globalPath,
		PathDisplay:  displayPath(globalPath),
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
