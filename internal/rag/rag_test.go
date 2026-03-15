package rag

import (
	"path/filepath"
	"strings"
	"testing"

	"github/xiuivfbc/NaturalCmd/internal/history"
)

func TestBuildHistoryContextReturnsRelevantEntries(t *testing.T) {
	store, err := history.Load(filepath.Join(t.TempDir(), "history.json"), 10)
	if err != nil {
		t.Fatalf("load history store: %v", err)
	}

	_ = store.Add("查看 git 分支", "git branch")
	_ = store.Add("查看 docker 容器", "docker ps")
	_ = store.Add("查看 git 状态", "git status")

	ctx := BuildHistoryContext("git", store, 2)
	if ctx == "" {
		t.Fatalf("expected non-empty context")
	}

	if !strings.Contains(ctx, "git") {
		t.Fatalf("expected git-related content, got: %s", ctx)
	}

	if strings.Contains(ctx, "docker ps") {
		t.Fatalf("expected docker entry filtered out for top 2 git results: %s", ctx)
	}
}

func TestBuildHistoryContextHandlesEmptyStore(t *testing.T) {
	store, err := history.Load(filepath.Join(t.TempDir(), "history.json"), 10)
	if err != nil {
		t.Fatalf("load history store: %v", err)
	}

	ctx := BuildHistoryContext("git", store, 3)
	if ctx != "" {
		t.Fatalf("expected empty context, got: %s", ctx)
	}
}

func TestBuildHistoryContextWithFeedbackPrefersSuccessfulCommand(t *testing.T) {
	store, err := history.Load(filepath.Join(t.TempDir(), "history.json"), 10)
	if err != nil {
		t.Fatalf("load history store: %v", err)
	}

	_ = store.Add("列出文件", "ls -la")
	_ = store.Add("列出文件", "find . -type f")

	feedbackStore, err := LoadFeedback(filepath.Join(t.TempDir(), "feedback.json"))
	if err != nil {
		t.Fatalf("load feedback store: %v", err)
	}

	_ = feedbackStore.RecordSuccess("find . -type f")
	_ = feedbackStore.RecordSuccess("find . -type f")
	_ = feedbackStore.RecordFailure("ls -la")

	ctx := BuildHistoryContextWithFeedback("列出 文件", store, feedbackStore, 1)
	if !strings.Contains(ctx, "find . -type f") {
		t.Fatalf("expected successful command to be preferred, got: %s", ctx)
	}
}

func TestFeedbackStoreRecordsWeight(t *testing.T) {
	feedbackStore, err := LoadFeedback(filepath.Join(t.TempDir(), "feedback.json"))
	if err != nil {
		t.Fatalf("load feedback store: %v", err)
	}

	_ = feedbackStore.RecordSuccess("git status")
	_ = feedbackStore.RecordSuccess("git status")
	_ = feedbackStore.RecordFailure("git status")

	if got := feedbackStore.WeightForScript("git status"); got != 1 {
		t.Fatalf("expected weight 1, got %d", got)
	}
}

func TestBuildHistoryContextChineseWithoutWhitespace(t *testing.T) {
	store, err := history.Load(filepath.Join(t.TempDir(), "history.json"), 10)
	if err != nil {
		t.Fatalf("load history store: %v", err)
	}

	_ = store.Add("查看远程仓库", "git remote -v")
	_ = store.Add("查看本地分支", "git branch")

	ctx := BuildHistoryContext("远程仓库", store, 1)
	if !strings.Contains(ctx, "git remote -v") {
		t.Fatalf("expected chinese no-whitespace query to match remote command, got: %s", ctx)
	}
}
