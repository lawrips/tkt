package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	configDirName  = ".tkt"
	configFileName = "config.yaml"
)

// Config stores tkt project configuration.
type Config struct {
	Projects map[string]ProjectConfig
}

// ProjectConfig stores per-project settings.
type ProjectConfig struct {
	Path         string `json:"path"`
	Store        string `json:"store"`
	AutoLink     bool   `json:"auto_link"`
	AutoClose    bool   `json:"auto_close"`
	RegisteredAt string `json:"registered_at"`
}

// Load reads ~/.tkt/config.yaml. Missing file returns empty config.
func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{Projects: map[string]ProjectConfig{}}, nil
		}
		return Config{}, err
	}

	cfg, err := parseConfig(string(raw))
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes ~/.tkt/config.yaml.
func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if cfg.Projects == nil {
		cfg.Projects = map[string]ProjectConfig{}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(marshalConfig(cfg)), 0644)
}

// ConfigPath returns ~/.tkt/config.yaml.
func ConfigPath() (string, error) {
	dir, err := configDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func configDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

// UpsertProject inserts or updates a project entry.
func (cfg *Config) UpsertProject(name string, project ProjectConfig) {
	if cfg.Projects == nil {
		cfg.Projects = map[string]ProjectConfig{}
	}
	cfg.Projects[name] = project
}

func parseConfig(raw string) (Config, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	cfg := Config{Projects: map[string]ProjectConfig{}}

	inProjects := false
	currentProject := ""

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		if line == "projects:" {
			inProjects = true
			currentProject = ""
			continue
		}
		if !inProjects {
			continue
		}

		indent := countLeadingSpaces(line)
		trimmed := strings.TrimSpace(line)

		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			currentProject = strings.TrimSuffix(trimmed, ":")
			if currentProject == "" {
				return Config{}, fmt.Errorf("empty project key in config")
			}
			if _, ok := cfg.Projects[currentProject]; !ok {
				cfg.Projects[currentProject] = ProjectConfig{}
			}
			continue
		}

		if indent == 4 && currentProject != "" {
			key, value, ok := splitKeyValue(trimmed)
			if !ok {
				return Config{}, fmt.Errorf("invalid config line: %q", line)
			}
			project := cfg.Projects[currentProject]
			switch key {
			case "path":
				project.Path = unquote(value)
			case "store":
				project.Store = unquote(value)
			case "registered_at":
				project.RegisteredAt = unquote(value)
			case "auto_link":
				b, err := strconv.ParseBool(unquote(value))
				if err != nil {
					return Config{}, fmt.Errorf("invalid auto_link value %q: %w", value, err)
				}
				project.AutoLink = b
			case "auto_close":
				b, err := strconv.ParseBool(unquote(value))
				if err != nil {
					return Config{}, fmt.Errorf("invalid auto_close value %q: %w", value, err)
				}
				project.AutoClose = b
			}
			cfg.Projects[currentProject] = project
		}
	}

	return cfg, nil
}

func marshalConfig(cfg Config) string {
	var b strings.Builder
	b.WriteString("projects:\n")

	keys := make([]string, 0, len(cfg.Projects))
	for key := range cfg.Projects {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		project := cfg.Projects[key]
		b.WriteString("  ")
		b.WriteString(key)
		b.WriteString(":\n")
		b.WriteString("    path: ")
		b.WriteString(project.Path)
		b.WriteString("\n")
		b.WriteString("    store: ")
		if project.Store == "" {
			b.WriteString("local\n")
		} else {
			b.WriteString(project.Store)
			b.WriteString("\n")
		}
		if project.RegisteredAt != "" {
			b.WriteString("    registered_at: ")
			b.WriteString(project.RegisteredAt)
			b.WriteString("\n")
		}
		b.WriteString("    auto_link: ")
		b.WriteString(strconv.FormatBool(project.AutoLink))
		b.WriteString("\n")
		b.WriteString("    auto_close: ")
		b.WriteString(strconv.FormatBool(project.AutoClose))
		b.WriteString("\n")
	}

	return b.String()
}

func splitKeyValue(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	return key, value, true
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1]
	}
	return value
}

func countLeadingSpaces(line string) int {
	count := 0
	for i := 0; i < len(line) && line[i] == ' '; i++ {
		count++
	}
	return count
}
