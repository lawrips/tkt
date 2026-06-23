package doctor

import (
	"fmt"
	"sort"
	"strings"
)

func FormatText(report Report) string {
	var b strings.Builder
	fmt.Fprintln(&b, "TKT Doctor")
	fmt.Fprintf(
		&b,
		"Overall: %s (%d pass, %d warn, %d fail)\n",
		strings.ToUpper(string(report.Status)),
		report.Summary.Pass,
		report.Summary.Warn,
		report.Summary.Fail,
	)

	if report.Project.Name != "" {
		fmt.Fprintf(&b, "Project: %s", report.Project.Name)
		if report.Project.Store != "" {
			fmt.Fprintf(&b, " (%s)", report.Project.Store)
		}
		if report.Project.TicketDir != "" {
			fmt.Fprintf(&b, " tickets=%s", report.Project.TicketDir)
		}
		fmt.Fprintln(&b)
	}

	byCategory := map[string][]Check{}
	categories := make([]string, 0)
	for _, check := range report.Checks {
		if _, ok := byCategory[check.Category]; !ok {
			categories = append(categories, check.Category)
		}
		byCategory[check.Category] = append(byCategory[check.Category], check)
	}
	sort.SliceStable(categories, func(i, j int) bool {
		return categoryOrder(categories[i]) < categoryOrder(categories[j])
	})

	for _, category := range categories {
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "%s:\n", categoryTitle(category))
		for _, check := range byCategory[category] {
			fmt.Fprintf(&b, "  %-4s %s\n", strings.ToUpper(string(check.Status)), check.Message)
			if check.Remediation != "" {
				fmt.Fprintf(&b, "       %s\n", check.Remediation)
			}
			if check.Command != "" {
				fmt.Fprintf(&b, "       command: %s\n", check.Command)
			}
		}
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func categoryOrder(category string) int {
	switch category {
	case "global":
		return 0
	case "project":
		return 1
	case "sync":
		return 2
	case "agent":
		return 3
	default:
		return 100
	}
}

func categoryTitle(category string) string {
	switch category {
	case "global":
		return "Global"
	case "project":
		return "Project"
	case "sync":
		return "Sync"
	case "agent":
		return "Agent"
	default:
		if category == "" {
			return "Other"
		}
		return strings.ToUpper(category[:1]) + category[1:]
	}
}
