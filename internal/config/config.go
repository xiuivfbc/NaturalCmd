package config

import (
	"os"
	"strconv"
	"strings"
)

// Config 定义配置结构
type Config struct {
	APIKey            string
	APIEndpoint       string
	Model             string
	Language          string
	SilentMode        bool
	Provider          string // 模型提供商: openai, aliyun
	SkillsEnabled     bool
	SkillsFile        string
	HistoryFile       string
	HistoryMax        int
	RAGEnabled        bool
	RAGTopK           int
	RAGMinLocalHit    int
	RAGMinLocalCover  float64
	RAGFeedbackFile   string
	RAGSemanticExpand bool

	// 主副模型配置 (支持跨厂商)
	ModelPrimary         string // 主模型名称 (e.g., "gpt-4o")
	ModelPrimaryProvider string // 主模型提供商 (openai, aliyun)
	ModelPrimaryKey      string // 主模型 API Key
	ModelPrimaryEndpoint string // 主模型 API Endpoint

	ModelSecondary         string // 副模型名称 (e.g., "gpt-4o-mini")
	ModelSecondaryProvider string // 副模型提供商 (openai, aliyun)
	ModelSecondaryKey      string // 副模型 API Key
	ModelSecondaryEndpoint string // 副模型 API Endpoint

	// 安全约束配置
	NegativeConstraintsEnabled bool   // 是否启用负向约束（默认启用）
	BlacklistPath              string // 黑名单文件路径
}

// Load 加载配置
func Load() (*Config, error) {
	// 主模型配置：如果没有指定，则使用原有的 MODEL/PROVIDER/API_KEY/API_ENDPOINT
	modelPrimary := getEnv("MODEL_PRIMARY", "")
	if modelPrimary == "" {
		modelPrimary = getEnv("MODEL", "gpt-4o-mini")
	}
	modelPrimaryProvider := getEnv("MODEL_PRIMARY_PROVIDER", "")
	if modelPrimaryProvider == "" {
		modelPrimaryProvider = getEnv("PROVIDER", "openai")
	}
	modelPrimaryKey := getEnv("MODEL_PRIMARY_KEY", "")
	if modelPrimaryKey == "" {
		modelPrimaryKey = getEnv("API_KEY", getEnv("OPENAI_KEY", ""))
	}
	modelPrimaryEndpoint := getEnv("MODEL_PRIMARY_ENDPOINT", "")
	if modelPrimaryEndpoint == "" {
		modelPrimaryEndpoint = getEnv("API_ENDPOINT", "https://api.openai.com/v1/chat/completions")
	}

	// 副模型配置：如果没有指定，则使用原有的 MODEL/PROVIDER/API_KEY/API_ENDPOINT
	modelSecondary := getEnv("MODEL_SECONDARY", "")
	if modelSecondary == "" {
		modelSecondary = modelPrimary
	}
	modelSecondaryProvider := getEnv("MODEL_SECONDARY_PROVIDER", "")
	if modelSecondaryProvider == "" {
		modelSecondaryProvider = modelPrimaryProvider
	}
	modelSecondaryKey := getEnv("MODEL_SECONDARY_KEY", "")
	if modelSecondaryKey == "" {
		modelSecondaryKey = modelPrimaryKey
	}
	modelSecondaryEndpoint := getEnv("MODEL_SECONDARY_ENDPOINT", "")
	if modelSecondaryEndpoint == "" {
		modelSecondaryEndpoint = modelPrimaryEndpoint
	}

	return &Config{
		APIKey:            modelPrimaryKey,      // 设置为主模型 Key，保持向后兼容性
		APIEndpoint:       modelPrimaryEndpoint, // 设置为主模型 Endpoint，保持向后兼容性
		Model:             modelPrimary,
		Language:          getEnv("LANGUAGE", "en"),
		SilentMode:        getEnv("SILENT_MODE", "false") == "true",
		Provider:          modelPrimaryProvider,
		SkillsEnabled:     getEnvAsBool("SKILLS_ENABLED", true),
		SkillsFile:        getEnv("SKILLS_FILE", "locales/skills.json"),
		HistoryFile:       getEnv("HISTORY_FILE", ""),
		HistoryMax:        getEnvAsInt("HISTORY_MAX_CAPACITY", 50),
		RAGEnabled:        getEnvAsBool("RAG_ENABLED", true),
		RAGTopK:           getEnvAsInt("RAG_TOP_K", 3),
		RAGMinLocalHit:    getEnvAsInt("RAG_MIN_LOCAL_HIT_SCORE", 4),
		RAGMinLocalCover:  getEnvAsFloat64("RAG_MIN_LOCAL_COVERAGE", 0.45),
		RAGFeedbackFile:   getEnv("RAG_FEEDBACK_FILE", ""),
		RAGSemanticExpand: getEnvAsBool("RAG_SEMANTIC_EXPAND", true),

		ModelPrimary:         modelPrimary,
		ModelPrimaryProvider: modelPrimaryProvider,
		ModelPrimaryKey:      modelPrimaryKey,
		ModelPrimaryEndpoint: modelPrimaryEndpoint,

		ModelSecondary:         modelSecondary,
		ModelSecondaryProvider: modelSecondaryProvider,
		ModelSecondaryKey:      modelSecondaryKey,
		ModelSecondaryEndpoint: modelSecondaryEndpoint,

		NegativeConstraintsEnabled: getEnvAsBool("ENABLE_NEGATIVE_CONSTRAINTS", true),
		BlacklistPath:              getEnv("BLACKLIST_PATH", "locales/safety_blacklist.json"),
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

func getEnvAsBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return defaultValue
	}

	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func getEnvAsFloat64(key string, defaultValue float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}

	if parsed < 0 {
		return 0
	}
	if parsed > 1 {
		return 1
	}

	return parsed
}
