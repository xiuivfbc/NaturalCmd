package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndResolveSkill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.json")
	content := `{
		"skills": [
			{
				"name": "project-check",
				"description": "run quality checks",
				"triggers": ["检查项目", "project check"],
				"commands": ["go test ./...", "go build ./..."],
				"continue_on_error": false
			}
		]
	}`

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write skills file: %v", err)
	}

	registry, err := Load(path)
	if err != nil {
		t.Fatalf("load skills: %v", err)
	}

	resolved, ok := registry.Resolve("请帮我检查项目并输出结果", "linux")
	if !ok {
		t.Fatalf("expected skill to match")
	}

	if resolved.Skill.Name != "project-check" {
		t.Fatalf("expected matched skill project-check, got %q", resolved.Skill.Name)
	}

	if resolved.Script != "go test ./... && go build ./..." {
		t.Fatalf("unexpected script: %q", resolved.Script)
	}
}

func TestResolveUsesPriorityWhenScoreTies(t *testing.T) {
	registry := &Registry{items: []Skill{
		{
			Name:     "basic",
			Triggers: []string{"发布"},
			Commands: []string{"echo basic"},
			Priority: 1,
		},
		{
			Name:     "advanced",
			Triggers: []string{"发布"},
			Commands: []string{"echo advanced"},
			Priority: 5,
		},
	}}

	resolved, ok := registry.Resolve("发布", "linux")
	if !ok {
		t.Fatalf("expected skill to match")
	}

	if resolved.Skill.Name != "advanced" {
		t.Fatalf("expected advanced skill due to higher priority, got %q", resolved.Skill.Name)
	}
}

func TestBuildCommandChainSeparators(t *testing.T) {
	if got := BuildCommandChain([]string{"echo 1", "echo 2"}, true, "windows"); got != "echo 1 & echo 2" {
		t.Fatalf("unexpected windows continue chain: %q", got)
	}

	if got := BuildCommandChain([]string{"echo 1", "echo 2"}, true, "linux"); got != "echo 1 ; echo 2" {
		t.Fatalf("unexpected unix continue chain: %q", got)
	}

	if got := BuildCommandChain([]string{"echo 1", "echo 2"}, false, "windows"); got != "echo 1 && echo 2" {
		t.Fatalf("unexpected fail-fast chain: %q", got)
	}
}

func TestBuildCatalogAndSelectedContext(t *testing.T) {
	registry := &Registry{items: []Skill{
		{
			Name:            "project-check",
			Description:     "run tests and build",
			Triggers:        []string{"检查项目"},
			Commands:        []string{"go test ./...", "go build ./..."},
			ContinueOnError: false,
			Priority:        10,
		},
		{
			Name:            "quick-clean",
			Commands:        []string{"go clean", "go mod tidy"},
			ContinueOnError: true,
		},
	}}

	catalog := registry.BuildCatalog()
	if !strings.Contains(catalog, "Available local skills") {
		t.Fatalf("expected catalog header, got: %s", catalog)
	}
	if !strings.Contains(catalog, "name=project-check") {
		t.Fatalf("expected project-check in catalog, got: %s", catalog)
	}

	ctx := registry.BuildSelectedContext([]string{"quick-clean", "not-exists", "quick-clean"}, "linux")
	if !strings.Contains(ctx, "quick-clean") {
		t.Fatalf("expected selected skill in context, got: %s", ctx)
	}
	if strings.Contains(ctx, "not-exists") {
		t.Fatalf("unexpected unknown skill in context, got: %s", ctx)
	}
	if !strings.Contains(ctx, "go clean ; go mod tidy") {
		t.Fatalf("expected linux command chain in context, got: %s", ctx)
	}
}
