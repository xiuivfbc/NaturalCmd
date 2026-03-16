package rag

import (
	"fmt"
	"sort"
	"strings"

	"github/xiuivfbc/NaturalCmd/internal/history"
	"github/xiuivfbc/NaturalCmd/internal/tokenizer"
)

const defaultTopK = 3

// BuildHistoryContext 从历史记录中检索与当前查询最相关的上下文，供模型生成命令时参考。
func BuildHistoryContext(query string, store *history.Store, topK int) string {
	return BuildHistoryContextWithFeedback(query, store, nil, topK)
}

// BuildHistoryContextWithFeedback 在历史检索基础上叠加执行反馈权重。
func BuildHistoryContextWithFeedback(query string, store *history.Store, feedback *FeedbackStore, topK int) string {
	if store == nil {
		return ""
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}

	entries := store.Search("")
	if len(entries) == 0 {
		return ""
	}

	if topK <= 0 {
		topK = defaultTopK
	}

	queryTokens := sliceToTokenSet(tokenizer.TokenizeForSearch(query))
	if len(queryTokens) == 0 {
		return ""
	}

	type scoredEntry struct {
		entry history.Entry
		score int
		idx   int
	}

	results := make([]scoredEntry, 0, len(entries))
	for index, entry := range entries {
		score := scoreEntry(queryTokens, entry, feedback)
		if score <= 0 {
			continue
		}
		results = append(results, scoredEntry{entry: entry, score: score, idx: index})
	}

	if len(results) == 0 {
		return ""
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			// 保持最近历史优先（Store.Search("") 已按最近在前）
			return results[i].idx < results[j].idx
		}
		return results[i].score > results[j].score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	var builder strings.Builder
	builder.WriteString("Relevant command history (reference only):\n")
	for i, item := range results {
		builder.WriteString(fmt.Sprintf("%d. User intent: %s\n", i+1, strings.TrimSpace(item.entry.Prompt)))
		builder.WriteString(fmt.Sprintf("   Command: %s\n", strings.TrimSpace(item.entry.Script)))
	}

	builder.WriteString("Use these as hints when appropriate, but output a command that best matches the current request.\n")
	return builder.String()
}

func scoreEntry(queryTokens map[string]struct{}, entry history.Entry, feedback *FeedbackStore) int {
	promptTokens := sliceToTokenSet(entry.PromptTokens)
	if len(promptTokens) == 0 {
		promptTokens = sliceToTokenSet(tokenizer.TokenizeForSearch(strings.TrimSpace(entry.Prompt)))
	}

	scriptTokens := sliceToTokenSet(entry.ScriptTokens)
	if len(scriptTokens) == 0 {
		scriptTokens = sliceToTokenSet(tokenizer.TokenizeForSearch(strings.TrimSpace(entry.Script)))
	}

	score := 0
	for token := range queryTokens {
		if _, ok := promptTokens[token]; ok {
			score += 3
		}
		if _, ok := scriptTokens[token]; ok {
			score += 1
		}
	}

	if feedback != nil {
		score += feedback.WeightForScript(entry.Script) * 2
	}

	return score
}

func sliceToTokenSet(tokens []string) map[string]struct{} {
	if len(tokens) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		value := strings.TrimSpace(strings.ToLower(token))
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}

	if len(set) == 0 {
		return nil
	}

	return set
}
