package ticket

// Frontmatter models the YAML metadata at the top of a ticket file.
// Extra preserves unknown keys so files can round-trip without schema loss.
type Frontmatter struct {
	ID          string   `yaml:"id,omitempty"`
	Status      string   `yaml:"status,omitempty"`
	Deps        []string `yaml:"deps,omitempty"`
	Links       []string `yaml:"links,omitempty"`
	Created     string   `yaml:"created,omitempty"`
	Type        string   `yaml:"type,omitempty"`
	Priority    int      `yaml:"priority,omitempty"`
	Assignee    string   `yaml:"assignee,omitempty"`
	Parent      string   `yaml:"parent,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	ExternalRef string   `yaml:"external_ref,omitempty"`
	Extra       map[string]ExtraField
}

// Document is a parsed ticket markdown document.
type Document struct {
	Frontmatter Frontmatter
	Body        string
}

// ExtraField preserves unknown frontmatter keys.
// If Block is true, Raw contains one or more indented lines as-is.
type ExtraField struct {
	Raw   string
	Block bool
}
