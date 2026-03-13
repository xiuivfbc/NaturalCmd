package config

import (
	"os"
	"strconv"
	"strings"
)

// Config 定义配置结构
type Config struct {
	APIKey      string
	APIEndpoint string
	Model       string
	Language    string
	SilentMode  bool
	Provider    string // 模型提供商: openai, aliyun
	HistoryFile string
	HistoryMax  int
}

// Load 加载配置
func Load() (*Config, error) {
	return &Config{
		APIKey:      getEnv("API_KEY", getEnv("OPENAI_KEY", "")),
		APIEndpoint: getEnv("API_ENDPOINT", getEnv("API_ENDPOINT", "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions")),
		Model:       getEnv("MODEL", "gpt-4o-mini"),
		Language:    getEnv("LANGUAGE", "en"),
		SilentMode:  getEnv("SILENT_MODE", "false") == "true",
		Provider:    getEnv("PROVIDER", "openai"),
		HistoryFile: getEnv("HISTORY_FILE", ""),
		HistoryMax:  getEnvAsInt("HISTORY_MAX_CAPACITY", 50),
	}, nil
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}
