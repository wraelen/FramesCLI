package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wraelen/framescli/internal/contracts"
)

func TestGeneratedAgentContractArtifactsAreCurrent(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	files, err := contracts.GeneratedFiles()
	if err != nil {
		t.Fatalf("render generated files: %v", err)
	}

	for _, file := range files {
		path := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Equal(got, file.Contents) {
			t.Fatalf("%s is stale; run ./scripts/generate-schemas.sh", file.Path)
		}
	}
}

func TestCLIResponseEnvelopeSchemaLeavesCommandUnconstrained(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "schemas", "cli-response-envelope.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing or invalid: %#v", schema["properties"])
	}
	command, ok := props["command"].(map[string]any)
	if !ok {
		t.Fatalf("command property missing or invalid: %#v", props["command"])
	}
	if got, ok := command["type"].(string); !ok || got != "string" {
		t.Fatalf("command property type=%#v, want string", command["type"])
	}
	if _, ok := command["enum"]; ok {
		t.Fatalf("command property should not enumerate CLI/MCP names: %#v", command["enum"])
	}
}

func TestActiveDocsDoNotReferenceRemovedTUISurface(t *testing.T) {
	tests := []struct {
		path      string
		forbidden []string
	}{
		{
			path: filepath.Join("..", "..", "docs", "AGENT_RECIPES.md"),
			forbidden: []string{
				"framescli tui",
			},
		},
		{
			path: filepath.Join("..", "..", "docs", "media", "README_MEDIA.md"),
			forbidden: []string{
				"hero-tui",
				"tui-overview",
				"framescli tui",
			},
		},
		{
			path: filepath.Join("..", "..", "scripts", "preflight.sh"),
			forbidden: []string{
				"./cmd/frames tui --help",
			},
		},
		{
			path: filepath.Join("..", "..", "CLAUDE.md"),
			forbidden: []string{
				"go test ./internal/tui",
				"internal/tui/",
			},
		},
		{
			path: filepath.Join("..", "..", "CHANGELOG.md"),
			forbidden: []string{
				"version subcommand",
			},
		},
	}

	for _, tc := range tests {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			raw, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.path, err)
			}
			text := string(raw)
			for _, needle := range tc.forbidden {
				if strings.Contains(text, needle) {
					t.Fatalf("%s still contains stale reference %q", tc.path, needle)
				}
			}
		})
	}
}
