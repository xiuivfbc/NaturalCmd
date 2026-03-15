package rag

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github/xiuivfbc/NaturalCmd/internal/history"
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

	queryTokens := tokenize(query)
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
	promptTokens := tokenize(strings.TrimSpace(entry.Prompt))
	scriptTokens := tokenize(strings.TrimSpace(entry.Script))

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

func tokenize(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	text = strings.ToLower(strings.TrimSpace(text))

	var latinBuilder strings.Builder
	hanRunes := make([]rune, 0, len(text))

	flushLatin := func() {
		if latinBuilder.Len() == 0 {
			return
		}
		token := strings.TrimSpace(latinBuilder.String())
		if len(token) > 1 {
			tokens[token] = struct{}{}
		}
		latinBuilder.Reset()
	}

	flushHan := func() {
		if len(hanRunes) == 0 {
			return
		}
		if len(hanRunes) >= 2 {
			// 连续中文片段作为一个整体 token
			tokens[string(hanRunes)] = struct{}{}
			// 同时生成双字 token，提升短句匹配能力
			for i := 0; i < len(hanRunes)-1; i++ {
				tokens[string(hanRunes[i:i+2])] = struct{}{}
			}
		}
		hanRunes = hanRunes[:0]
	}

	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			hanRunes = append(hanRunes, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			flushHan()
			latinBuilder.WriteRune(r)
		default:
			flushLatin()
			flushHan()
		}
	}

	flushLatin()
	flushHan()

	return tokens
}
