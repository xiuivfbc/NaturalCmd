package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
)

const defaultSkillsFile = "skills.json"

// Registry 保存已加载的自定义技能。
type Registry struct {
	items []Skill
}

// HasSkills 返回当前注册表是否包含可用技能。
func (r *Registry) HasSkills() bool {
	return r != nil && len(r.items) > 0
}

// BuildCatalog 生成可提供给模型的技能目录描述。
func (r *Registry) BuildCatalog() string {
	if r == nil || len(r.items) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Available local skills:\n")
	for index, item := range r.items {
		builder.WriteString(fmt.Sprintf("%d) name=%s\n", index+1, item.Name))
		if item.Description != "" {
			builder.WriteString(fmt.Sprintf("   description=%s\n", item.Description))
		}
		if len(item.Triggers) > 0 {
			builder.WriteString(fmt.Sprintf("   triggers=%s\n", strings.Join(item.Triggers, ", ")))
		}
		if len(item.Commands) > 0 {
			builder.WriteString("   commands:\n")
			for commandIndex, command := range item.Commands {
				builder.WriteString(fmt.Sprintf("   - [%d] %s\n", commandIndex+1, command))
			}
		}
		builder.WriteString(fmt.Sprintf("   continue_on_error=%t\n", item.ContinueOnError))
		builder.WriteString(fmt.Sprintf("   priority=%d\n", item.Priority))
	}

	return builder.String()
}

// BuildSelectedContext 根据模型返回的技能名构建增强上下文。
func (r *Registry) BuildSelectedContext(selectedNames []string, goos string) string {
	if r == nil || len(r.items) == 0 || len(selectedNames) == 0 {
		return ""
	}

	selected := make([]Skill, 0, len(selectedNames))
	seen := make(map[string]struct{}, len(selectedNames))
	for _, name := range selectedNames {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		for _, item := range r.items {
			if strings.EqualFold(item.Name, key) {
				selected = append(selected, item)
				break
			}
		}
	}

	if len(selected) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Skill planning context (selected by model):\n")
	for i, item := range selected {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.Name))
		if item.Description != "" {
			builder.WriteString(fmt.Sprintf("   description: %s\n", item.Description))
		}
		chain := BuildCommandChain(item.Commands, item.ContinueOnError, goos)
		if chain != "" {
			builder.WriteString(fmt.Sprintf("   suggested-chain: %s\n", chain))
		}
	}
	builder.WriteString("Use these selected skills as actionable references to compose the best final command for the current user request.\n")

	return builder.String()
}

// Skill 表示一个可匹配的命令集合。
type Skill struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Triggers        []string `json:"triggers"`
	Commands        []string `json:"commands"`
	ContinueOnError bool     `json:"continue_on_error"`
	Priority        int      `json:"priority"`
}

// Resolved 表示一次命中的技能及生成结果。
type Resolved struct {
	Skill     Skill
	Script    string
	MatchedBy string
}

type skillsFile struct {
	Skills []Skill `json:"skills"`
}

// Load 从文件加载技能定义。文件不存在时返回空注册表。
func Load(path string) (*Registry, error) {
	resolvedPath := strings.TrimSpace(path)
	if resolvedPath == "" {
		resolvedPath = defaultSkillsFile
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Registry{}, nil
		}
		return nil, fmt.Errorf("read skills file: %w", err)
	}

	if strings.TrimSpace(string(content)) == "" {
		return &Registry{}, nil
	}

	var payload skillsFile
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("parse skills file: %w", err)
	}

	items := make([]Skill, 0, len(payload.Skills))
	for _, item := range payload.Skills {
		normalized, ok := normalizeSkill(item)
		if !ok {
			continue
		}
		items = append(items, normalized)
	}

	return &Registry{items: items}, nil
}

// Resolve 尝试根据一句话匹配技能，并返回命令集合拼接后的脚本。
func (r *Registry) Resolve(prompt string, goos string) (Resolved, bool) {
	if r == nil || len(r.items) == 0 {
		return Resolved{}, false
	}

	query := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(prompt)), " "))
	if query == "" {
		return Resolved{}, false
	}

	type candidate struct {
		skill     Skill
		matchedBy string
		score     int
	}

	candidates := make([]candidate, 0, len(r.items))
	for _, item := range r.items {
		if score, trigger, ok := matchSkill(query, item); ok {
			candidates = append(candidates, candidate{skill: item, matchedBy: trigger, score: score})
		}
	}

	if len(candidates) == 0 {
		return Resolved{}, false
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].skill.Priority != candidates[j].skill.Priority {
			return candidates[i].skill.Priority > candidates[j].skill.Priority
		}
		return strings.ToLower(candidates[i].skill.Name) < strings.ToLower(candidates[j].skill.Name)
	})

	chosen := candidates[0]
	script := BuildCommandChain(chosen.skill.Commands, chosen.skill.ContinueOnError, goos)
	if script == "" {
		return Resolved{}, false
	}

	return Resolved{
		Skill:     chosen.skill,
		Script:    script,
		MatchedBy: chosen.matchedBy,
	}, true
}

// ResolveCurrentOS 使用当前运行平台进行技能解析。
func (r *Registry) ResolveCurrentOS(prompt string) (Resolved, bool) {
	return r.Resolve(prompt, runtime.GOOS)
}

func normalizeSkill(item Skill) (Skill, bool) {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.Commands = normalizeValues(item.Commands)
	item.Triggers = normalizeValues(item.Triggers)

	if item.Name == "" {
		return Skill{}, false
	}

	if len(item.Commands) == 0 {
		return Skill{}, false
	}

	if len(item.Triggers) == 0 {
		item.Triggers = []string{item.Name}
	}

	return item, true
}

func normalizeValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func matchSkill(query string, item Skill) (int, string, bool) {
	bestScore := 0
	bestTrigger := ""
	for _, trigger := range item.Triggers {
		normalizedTrigger := strings.ToLower(trigger)
		if len([]rune(normalizedTrigger)) <= 1 {
			continue
		}

		score := 0
		switch {
		case query == normalizedTrigger:
			score = 100
		case strings.Contains(query, normalizedTrigger):
			score = 60
		}

		if score > bestScore {
			bestScore = score
			bestTrigger = trigger
		}
	}

	if bestScore == 0 {
		return 0, "", false
	}

	return bestScore, bestTrigger, true
}

// BuildCommandChain 将技能中的命令集合拼接为单行脚本。
func BuildCommandChain(commands []string, continueOnError bool, goos string) string {
	parts := make([]string, 0, len(commands))
	for _, command := range commands {
		value := strings.TrimSpace(command)
		if value == "" {
			continue
		}
		parts = append(parts, value)
	}

	if len(parts) == 0 {
		return ""
	}

	if len(parts) == 1 {
		return parts[0]
	}

	separator := " && "
	if continueOnError {
		if goos == "windows" {
			separator = " & "
		} else {
			separator = " ; "
		}
	}

	return strings.Join(parts, separator)
}
