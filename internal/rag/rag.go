package rag

import (
	"fmt"
	"sort"
	"strings"

	"github/xiuivfbc/NaturalCmd/internal/history"
	"github/xiuivfbc/NaturalCmd/internal/tokenizer"
)

const defaultTopK = 3

// MatchResult 表示一次历史检索的结果及其命中分值。
type MatchResult struct {
	Context   string
	BestScore int
	Coverage  float64
}

// BuildHistoryContext 从历史记录中检索与当前查询最相关的上下文，供模型生成命令时参考。
func BuildHistoryContext(query string, store *history.Store, topK int) string {
	return BuildHistoryContextWithFeedback(query, store, nil, topK)
}

// BuildHistoryContextWithFeedback 在历史检索基础上叠加执行反馈权重。
func BuildHistoryContextWithFeedback(query string, store *history.Store, feedback *FeedbackStore, topK int) string {
	return BuildHistoryMatchWithFeedback(query, store, feedback, topK).Context
}

// BuildHistoryMatchWithFeedback 在历史检索基础上叠加执行反馈权重，并返回命中分值。
func BuildHistoryMatchWithFeedback(query string, store *history.Store, feedback *FeedbackStore, topK int) MatchResult {
	if store == nil {
		return MatchResult{}
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return MatchResult{}
	}

	entries := store.Search("")
	if len(entries) == 0 {
		return MatchResult{}
	}

	if topK <= 0 {
		topK = defaultTopK
	}

	queryTokens := sliceToTokenSet(tokenizer.TokenizeForSearch(query))
	if len(queryTokens) == 0 {
		return MatchResult{}
	}
	totalQueryTokens := len(queryTokens)

	type scoredEntry struct {
		entry    history.Entry
		score    int
		coverage float64
		idx      int
	}

	results := make([]scoredEntry, 0, len(entries))
	for index, entry := range entries {
		score, matchedCount := scoreEntry(queryTokens, entry, feedback)
		if score <= 0 {
			continue
		}
		coverage := 0.0
		if totalQueryTokens > 0 {
			coverage = float64(matchedCount) / float64(totalQueryTokens)
		}
		results = append(results, scoredEntry{entry: entry, score: score, coverage: coverage, idx: index})
	}

	if len(results) == 0 {
		return MatchResult{}
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
	return MatchResult{
		Context:   builder.String(),
		BestScore: results[0].score,
		Coverage:  results[0].coverage,
	}
}

func scoreEntry(queryTokens map[string]struct{}, entry history.Entry, feedback *FeedbackStore) (int, int) {
	promptTokens := sliceToTokenSet(entry.PromptTokens)
	if len(promptTokens) == 0 {
		promptTokens = sliceToTokenSet(tokenizer.TokenizeForSearch(strings.TrimSpace(entry.Prompt)))
	}

	scriptTokens := sliceToTokenSet(entry.ScriptTokens)
	if len(scriptTokens) == 0 {
		scriptTokens = sliceToTokenSet(tokenizer.TokenizeForSearch(strings.TrimSpace(entry.Script)))
	}

	score := 0
	matchedCount := 0
	for token := range queryTokens {
		matched := false
		if _, ok := promptTokens[token]; ok {
			score += 3
			matched = true
		}
		if _, ok := scriptTokens[token]; ok {
			score += 1
			matched = true
		}
		if matched {
			matchedCount++
		}
	}

	if feedback != nil {
		score += feedback.WeightForScript(entry.Script) * 2
	}

	return score, matchedCount
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
