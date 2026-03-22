package tokenizer

import (
	"strings"
	"sync"
	"unicode"

	"github.com/go-ego/gse"
)

var (
	segmenter     gse.Segmenter
	segmenterOnce sync.Once
)

// TokenizeForSearch 生成用于检索的 token 集合（去重后返回）。
func TokenizeForSearch(text string) []string {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return nil
	}

	tokenSet := make(map[string]struct{})
	add := func(token string) {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" {
			return
		}

		runes := []rune(token)
		if len(runes) < 2 {
			return
		}

		tokenSet[token] = struct{}{}
	}

	var latinBuilder strings.Builder
	hanRunes := make([]rune, 0, len(text))

	flushLatin := func() {
		if latinBuilder.Len() == 0 {
			return
		}
		add(latinBuilder.String())
		latinBuilder.Reset()
	}

	flushHan := func() {
		if len(hanRunes) == 0 {
			return
		}

		chunk := string(hanRunes)
		for _, token := range cutChinese(chunk) {
			add(token)
		}

		add(string(hanRunes))
		for i := 0; i < len(hanRunes)-1; i++ {
			add(string(hanRunes[i : i+2]))
		}
		for i := 0; i < len(hanRunes)-2; i++ {
			add(string(hanRunes[i : i+3]))
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

	if len(tokenSet) == 0 {
		return nil
	}

	tokens := make([]string, 0, len(tokenSet))
	for token := range tokenSet {
		tokens = append(tokens, token)
	}

	return tokens
}

func cutChinese(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	defer func() {
		_ = recover()
	}()

	segmenterOnce.Do(func() {
		segmenter.SkipLog = true
		segmenter.LoadDict()
	})

	return segmenter.CutSearch(text, true)
}
