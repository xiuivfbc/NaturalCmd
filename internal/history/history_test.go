package history

import (
	"path/filepath"
	"testing"
)

func TestStoreAddUsesLRUCapacity(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "history.json"), 2)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}

	if err := store.Add("first prompt", "echo first"); err != nil {
		t.Fatalf("add first: %v", err)
	}
	if err := store.Add("second prompt", "echo second"); err != nil {
		t.Fatalf("add second: %v", err)
	}
	if err := store.Add("first prompt", "echo first updated"); err != nil {
		t.Fatalf("refresh first: %v", err)
	}
	if err := store.Add("third prompt", "echo third"); err != nil {
		t.Fatalf("add third: %v", err)
	}

	entries := store.Search("")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Prompt != "third prompt" {
		t.Fatalf("expected most recent entry to be third prompt, got %q", entries[0].Prompt)
	}
	if entries[1].Prompt != "first prompt" {
		t.Fatalf("expected refreshed entry to be retained, got %q", entries[1].Prompt)
	}
	if entries[1].Script != "echo first updated" {
		t.Fatalf("expected refreshed script to be updated, got %q", entries[1].Script)
	}
}

func TestStoreSearchMatchesPromptAndScript(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "history.json"), 5)
	if err != nil {
		t.Fatalf("load store: %v", err)
	}

	_ = store.Add("list containers", "docker ps")
	_ = store.Add("show branches", "git branch")

	byPrompt := store.Search("container")
	if len(byPrompt) != 1 || byPrompt[0].Prompt != "list containers" {
		t.Fatalf("unexpected prompt search result: %+v", byPrompt)
	}

	byScript := store.Search("git branch")
	if len(byScript) != 1 || byScript[0].Prompt != "show branches" {
		t.Fatalf("unexpected script search result: %+v", byScript)
	}

	all := store.Search("")
	if len(all) != 2 {
		t.Fatalf("expected all entries when query is empty, got %d", len(all))
	}
}
