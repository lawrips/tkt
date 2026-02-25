package ticket

import "strings"

// Body represents structured markdown sections of a ticket.
type Body struct {
	Title              string
	Description        string
	Design             string
	AcceptanceCriteria string
	OtherSections      []Section
}

// Section is an additional markdown heading block preserved during edits.
type Section struct {
	Heading string
	Content string
}

// ParseBody parses a ticket markdown body into known sections plus extras.
func ParseBody(markdown string) Body {
	text := strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	out := Body{}
	if len(lines) == 0 {
		return out
	}

	i := 0
	if strings.HasPrefix(lines[0], "# ") {
		out.Title = strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
		i = 1
	}

	descriptionLines := make([]string, 0)
	currentHeading := ""
	currentLines := make([]string, 0)

	flush := func() {
		content := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if currentHeading == "" {
			descriptionLines = append(descriptionLines, strings.Join(currentLines, "\n"))
		} else {
			switch strings.ToLower(currentHeading) {
			case "design":
				out.Design = content
			case "acceptance criteria":
				out.AcceptanceCriteria = content
			default:
				out.OtherSections = append(out.OtherSections, Section{
					Heading: currentHeading,
					Content: content,
				})
			}
		}
		currentLines = currentLines[:0]
	}

	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "## ") {
			flush()
			currentHeading = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		currentLines = append(currentLines, line)
	}
	flush()

	out.Description = strings.TrimSpace(strings.Join(descriptionLines, "\n"))
	return out
}

// RenderBody formats structured ticket body content back into markdown.
func RenderBody(body Body) string {
	var b strings.Builder

	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = "Untitled ticket"
	}

	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n\n")

	appendSectionContent(&b, strings.TrimSpace(body.Description))
	appendHeadingSection(&b, "Design", strings.TrimSpace(body.Design))
	appendHeadingSection(&b, "Acceptance Criteria", strings.TrimSpace(body.AcceptanceCriteria))

	for _, section := range body.OtherSections {
		appendHeadingSection(&b, section.Heading, strings.TrimSpace(section.Content))
	}

	out := strings.TrimRight(b.String(), "\n")
	return out + "\n"
}

func appendSectionContent(b *strings.Builder, content string) {
	if content == "" {
		return
	}
	b.WriteString(content)
	b.WriteString("\n\n")
}

func appendHeadingSection(b *strings.Builder, heading string, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	b.WriteString("## ")
	b.WriteString(strings.TrimSpace(heading))
	b.WriteString("\n\n")
	b.WriteString(content)
	b.WriteString("\n\n")
}
