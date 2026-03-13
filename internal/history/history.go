package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultCapacity = 50

// Entry 表示一条历史记录。
type Entry struct {
	Prompt    string    `json:"prompt"`
	Script    string    `json:"script"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store 管理历史记录，并使用 LRU 规则裁剪容量。
type Store struct {
	path     string
	capacity int
	entries  []Entry
}

// Load 从磁盘加载历史记录。
func Load(path string, capacity int) (*Store, error) {
	if path == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = defaultPath
	}

	store := &Store{
		path:     path,
		capacity: normalizeCapacity(capacity),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return store, nil
	}

	if err := json.Unmarshal(data, &store.entries); err != nil {
		return nil, err
	}

	store.entries = normalizeEntries(store.entries, store.capacity)
	return store, nil
}

// DefaultPath 返回默认历史文件路径。
func DefaultPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".github/xiuivfbc/NaturalCmd_history.json"), nil
}

// Add 新增或更新一条历史记录，并按 LRU 规则移动到最前。
func (s *Store) Add(prompt, script string) error {
	prompt = strings.TrimSpace(prompt)
	script = strings.TrimSpace(script)
	if prompt == "" {
		return nil
	}

	entry := Entry{
		Prompt:    prompt,
		Script:    script,
		UpdatedAt: time.Now(),
	}

	entries := make([]Entry, 0, len(s.entries)+1)
	entries = append(entries, entry)

	entryKey := normalizeKey(prompt)
	for _, existing := range s.entries {
		if normalizeKey(existing.Prompt) == entryKey {
			continue
		}
		entries = append(entries, existing)
	}

	s.entries = normalizeEntries(entries, s.capacity)
	return s.save()
}

// Search 按关键字进行简单搜索，返回按最近使用排序的历史结果。
func (s *Store) Search(query string) []Entry {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return cloneEntries(s.entries)
	}

	results := make([]Entry, 0)
	for _, entry := range s.entries {
		prompt := strings.ToLower(entry.Prompt)
		script := strings.ToLower(entry.Script)
		if strings.Contains(prompt, query) || strings.Contains(script, query) {
			results = append(results, entry)
		}
	}

	return results
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0o600)
}

func normalizeEntries(entries []Entry, capacity int) []Entry {
	capacity = normalizeCapacity(capacity)
	seen := make(map[string]struct{}, len(entries))
	result := make([]Entry, 0, min(len(entries), capacity))

	for _, entry := range entries {
		prompt := strings.TrimSpace(entry.Prompt)
		if prompt == "" {
			continue
		}

		key := normalizeKey(prompt)
		if _, exists := seen[key]; exists {
			continue
		}

		entry.Prompt = prompt
		entry.Script = strings.TrimSpace(entry.Script)
		seen[key] = struct{}{}
		result = append(result, entry)

		if len(result) == capacity {
			break
		}
	}

	return result
}

func normalizeCapacity(capacity int) int {
	if capacity <= 0 {
		return defaultCapacity
	}
	return capacity
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cloneEntries(entries []Entry) []Entry {
	cloned := make([]Entry, len(entries))
	copy(cloned, entries)
	return cloned
}
