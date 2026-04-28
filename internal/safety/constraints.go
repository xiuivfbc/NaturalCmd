package safety

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// DangerousCommand represents a dangerous keyword and its potential consequence
type DangerousCommand struct {
	Keyword     string `json:"keyword"`
	Consequence string `json:"consequence"`
}

// BlacklistConfig represents the structure of the blacklist file
type BlacklistConfig struct {
	DangerousCommands []DangerousCommand `json:"dangerous_commands"`
}

var blacklist BlacklistConfig

// InitBlacklist loads the safety blacklist from file
func InitBlacklist(blacklistPath string) error {
	data, err := os.ReadFile(blacklistPath)
	if err != nil {
		// If file doesn't exist, return without error (safety disabled)
		return nil
	}

	err = json.Unmarshal(data, &blacklist)
	if err != nil {
		return fmt.Errorf("failed to parse blacklist file: %w", err)
	}

	return nil
}

// DetectedDanger represents a detected dangerous operation
type DetectedDanger struct {
	Keyword     string
	Consequence string
}

// CheckCommand checks if a command contains dangerous operations
// Returns a list of detected dangerous keywords and their consequences
func CheckCommand(cmd string) []DetectedDanger {
	var dangers []DetectedDanger

	cmdLower := strings.ToLower(cmd)

	for _, dc := range blacklist.DangerousCommands {
		keyword := strings.ToLower(dc.Keyword)

		// Determine matching strategy based on keyword content
		var matched bool

		// If keyword contains spaces or special patterns, use substring matching
		// (e.g., "chmod 777", "cipher /w", "> /dev/sda")
		if strings.Contains(keyword, " ") || strings.Contains(keyword, "/") ||
			strings.Contains(keyword, ":") || strings.Contains(keyword, "(") {
			// Multi-word or special pattern: direct substring matching
			matched = strings.Contains(cmdLower, keyword)
		} else if keyword == ">" {
			// Special case: ">" is a single character, match as substring
			matched = strings.Contains(cmdLower, keyword)
		} else {
			// Single word: use word boundary to avoid false positives
			// e.g., "rm" should not match "arm-linux-gcc" or "trim"
			pattern := `\b` + regexp.QuoteMeta(keyword) + `\b`
			if r, err := regexp.Compile(pattern); err == nil {
				matched = r.MatchString(cmdLower)
			} else {
				// Fallback to substring matching if regex fails
				matched = strings.Contains(cmdLower, keyword)
			}
		}

		if matched {
			// Avoid duplicates
			isDuplicate := false
			for _, d := range dangers {
				if d.Keyword == dc.Keyword {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				dangers = append(dangers, DetectedDanger{
					Keyword:     dc.Keyword,
					Consequence: dc.Consequence,
				})
			}
		}
	}

	return dangers
}

// HasDangerousOperations checks if a command contains any dangerous operations
func HasDangerousOperations(cmd string) bool {
	return len(CheckCommand(cmd)) > 0
}
