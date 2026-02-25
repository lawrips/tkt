package ticket

import "testing"

func TestParseBodyFullDocument(t *testing.T) {
	md := "# My Ticket\n\nSome description.\n\n## Design\n\nUse approach X.\n\n## Acceptance Criteria\n\n- Works\n"
	body := ParseBody(md)

	if body.Title != "My Ticket" {
		t.Fatalf("Title: want %q, got %q", "My Ticket", body.Title)
	}
	if body.Description != "Some description." {
		t.Fatalf("Description: want %q, got %q", "Some description.", body.Description)
	}
	if body.Design != "Use approach X." {
		t.Fatalf("Design: want %q, got %q", "Use approach X.", body.Design)
	}
	if body.AcceptanceCriteria != "- Works" {
		t.Fatalf("AcceptanceCriteria: want %q, got %q", "- Works", body.AcceptanceCriteria)
	}
}

func TestParseBodyNoTitle(t *testing.T) {
	md := "Just a description with no heading.\n"
	body := ParseBody(md)

	if body.Title != "" {
		t.Fatalf("Title: want empty, got %q", body.Title)
	}
	if body.Description != "Just a description with no heading." {
		t.Fatalf("Description: want %q, got %q", "Just a description with no heading.", body.Description)
	}
}

func TestParseBodyEmpty(t *testing.T) {
	body := ParseBody("")

	if body.Title != "" {
		t.Fatalf("Title: want empty, got %q", body.Title)
	}
	if body.Description != "" {
		t.Fatalf("Description: want empty, got %q", body.Description)
	}
}

func TestParseBodyCRLF(t *testing.T) {
	md := "# Title\r\n\r\nDescription.\r\n\r\n## Design\r\n\r\nDesign text.\r\n"
	body := ParseBody(md)

	if body.Title != "Title" {
		t.Fatalf("Title: want %q, got %q", "Title", body.Title)
	}
	if body.Description != "Description." {
		t.Fatalf("Description: want %q, got %q", "Description.", body.Description)
	}
	if body.Design != "Design text." {
		t.Fatalf("Design: want %q, got %q", "Design text.", body.Design)
	}
}

func TestParseBodyUnknownSections(t *testing.T) {
	md := "# Ticket\n\nDesc.\n\n## Notes\n\nSome notes.\n\n## References\n\nSome refs.\n"
	body := ParseBody(md)

	if len(body.OtherSections) != 2 {
		t.Fatalf("OtherSections: want 2, got %d", len(body.OtherSections))
	}
	if body.OtherSections[0].Heading != "Notes" {
		t.Fatalf("OtherSections[0].Heading: want Notes, got %q", body.OtherSections[0].Heading)
	}
	if body.OtherSections[0].Content != "Some notes." {
		t.Fatalf("OtherSections[0].Content: want %q, got %q", "Some notes.", body.OtherSections[0].Content)
	}
	if body.OtherSections[1].Heading != "References" {
		t.Fatalf("OtherSections[1].Heading: want References, got %q", body.OtherSections[1].Heading)
	}
}

func TestParseBodyTitleOnly(t *testing.T) {
	body := ParseBody("# Just a title\n")

	if body.Title != "Just a title" {
		t.Fatalf("Title: want %q, got %q", "Just a title", body.Title)
	}
	if body.Description != "" {
		t.Fatalf("Description: want empty, got %q", body.Description)
	}
}

func TestRenderBodyUntitledFallback(t *testing.T) {
	body := Body{Description: "Some content."}
	got := RenderBody(body)

	want := "# Untitled ticket\n\nSome content.\n"
	if got != want {
		t.Fatalf("want:\n%s\ngot:\n%s", want, got)
	}
}

func TestRenderBodySkipsEmptySections(t *testing.T) {
	body := Body{Title: "Test", Description: "Desc."}
	got := RenderBody(body)

	want := "# Test\n\nDesc.\n"
	if got != want {
		t.Fatalf("want:\n%s\ngot:\n%s", want, got)
	}
}

func TestRenderBodyRoundTrip(t *testing.T) {
	original := "# Round Trip\n\nDescription here.\n\n## Design\n\nDesign notes.\n\n## Acceptance Criteria\n\n- Criterion 1\n- Criterion 2\n\n## Notes\n\nExtra section.\n"
	body := ParseBody(original)
	rendered := RenderBody(body)

	if rendered != original {
		t.Fatalf("round-trip mismatch:\nwant:\n%s\ngot:\n%s", original, rendered)
	}
}
