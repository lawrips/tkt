package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name          string
		explicit      string
		moduleVersion string
		want          string
	}{
		{name: "linker version wins", explicit: "v1.2.3", moduleVersion: "v9.9.9", want: "v1.2.3"},
		{name: "tagged go install version", explicit: "dev", moduleVersion: "v1.2.3", want: "v1.2.3"},
		{name: "prerelease module version", explicit: "dev", moduleVersion: "v1.2.3-rc.1", want: "v1.2.3-rc.1"},
		{name: "local build", explicit: "dev", moduleVersion: "(devel)", want: "dev"},
		{name: "missing build data", explicit: "", moduleVersion: "", want: "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.explicit, tt.moduleVersion); got != tt.want {
				t.Fatalf("resolveVersion(%q, %q) = %q, want %q", tt.explicit, tt.moduleVersion, got, tt.want)
			}
		})
	}
}

func TestVersionJSONReportsResolvedVersion(t *testing.T) {
	previous := versionString
	t.Cleanup(func() { versionString = previous })
	SetVersion("v1.2.3")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run([]string{"version", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("run version --json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode version JSON: %v", err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok || data["version"] != "v1.2.3" {
		t.Fatalf("unexpected version payload: %#v", payload)
	}
}
