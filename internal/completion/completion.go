package completion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github/xiuivfbc/NaturalCmd/internal/config"
	"github/xiuivfbc/NaturalCmd/internal/utils"
)

// 全局 i18n bundle
var bundle *i18n.Bundle

// 初始化 i18n bundle
func init() {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	// 加载翻译文件
	_, err := bundle.LoadMessageFile("locales/en.json")
	if err != nil {
		fmt.Printf("Error loading en.json: %v\n", err)
	}
	_, err = bundle.LoadMessageFile("locales/zh.json")
	if err != nil {
		fmt.Printf("Error loading zh.json: %v\n", err)
	}
}

// GenerateScript 生成shell脚本
func GenerateScript(prompt string, cfg *config.Config) (string, error) {
	// 模拟模式：直接返回预设命令
	if cfg.APIKey == "mock" {
		fmt.Println("```ls -la```")
		return "ls -la", nil
	}

	fullPrompt := buildFullPrompt(prompt, cfg)

	messages := []Message{
		{Role: "user", Content: fullPrompt},
	}

	var result string
	var err error

	// 根据提供商选择不同的API调用
	switch cfg.Provider {
	case "aliyun":
		request := AliyunRequest{
			Model:       cfg.Model,
			Messages:    messages,
			Stream:      false,
			Temperature: 0.7,
			TopP:        0.95,
		}
		result, err = callAliyun(request, cfg)
	default: // openai
		request := OpenAIRequest{
			Model:    cfg.Model,
			Messages: messages,
			Stream:   true,
		}
		result, err = callOpenAI(request, cfg)
	}

	if err != nil {
		return "", err
	}

	// 提取命令，移除代码块标记
	script := utils.ExtractScript(result)
	return script, nil
}

// GenerateExplanation 生成脚本解释
func GenerateExplanation(script string, cfg *config.Config) (string, error) {
	// 模拟模式
	if cfg.APIKey == "mock" {
		// 创建本地化器
		localizer := i18n.NewLocalizer(bundle, cfg.Language)

		// 使用翻译
		explanation := localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "mockExplanation",
		})
		fmt.Println(explanation)
		return explanation, nil
	}

	// 创建本地化器
	localizer := i18n.NewLocalizer(bundle, cfg.Language)

	// 使用翻译
	prompt := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "generateExplanationPrompt",
		TemplateData: map[string]interface{}{
			"Script": script,
		},
	})

	messages := []Message{
		{Role: "user", Content: prompt},
	}

	var result string
	var err error

	// 根据提供商选择不同的API调用
	switch cfg.Provider {
	case "aliyun":
		request := AliyunRequest{
			Model:       cfg.Model,
			Messages:    messages,
			Stream:      false,
			Temperature: 0.7,
			TopP:        0.95,
		}
		result, err = callAliyun(request, cfg)
	default: // openai
		request := OpenAIRequest{
			Model:    cfg.Model,
			Messages: messages,
			Stream:   true,
		}
		result, err = callOpenAI(request, cfg)
	}

	return result, err
}

// callOpenAI 调用OpenAI API
func callOpenAI(request OpenAIRequest, cfg *config.Config) (string, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", cfg.APIEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.APIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error: %s", string(body))
	}

	// 处理流式响应
	var result string
	reader := resp.Body

	buffer := make([]byte, 1024)
	for {
		n, err := reader.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}

		data := string(buffer[:n])
		// 解析SSE格式的响应
		lines := bytes.Split([]byte(data), []byte("\n"))
		for _, line := range lines {
			line = bytes.TrimSpace(line)
			if len(line) == 0 || bytes.HasPrefix(line, []byte(":")) {
				continue
			}

			if bytes.HasPrefix(line, []byte("data: ")) {
				line = bytes.TrimPrefix(line, []byte("data: "))
				if string(line) == "[DONE]" {
					goto done
				}

				var response OpenAIResponse
				if err := json.Unmarshal(line, &response); err != nil {
					continue
				}

				for _, choice := range response.Choices {
					content := choice.Delta.Content
					result += content
					fmt.Print(content)
				}
			}
		}
	}

done:
	fmt.Println()
	return result, nil
}

// callAliyun 调用阿里云大模型API
func callAliyun(request AliyunRequest, cfg *config.Config) (string, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", cfg.APIEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.APIKey))

	// 设置超时
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Aliyun API error: %s", string(body))
	}

	// 处理响应
	var result string
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 尝试解析为OpenAI兼容响应
	var openAIResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &openAIResponse); err == nil {
		// 处理OpenAI兼容响应
		for _, choice := range openAIResponse.Choices {
			result += choice.Message.Content
		}
	} else {
		// 直接返回原始响应
		result = string(body)
	}

	return result, nil
}

// buildFullPrompt 构建完整的提示
func buildFullPrompt(prompt string, cfg *config.Config) string {
	shell := utils.GetShell()
	os := utils.GetOS()

	// 获取当前环境信息
	envInfo := "\n\nCurrent environment:\n"

	// 获取当前工作目录
	currentDir, err := utils.GetCurrentDir()
	if err == nil {
		envInfo += fmt.Sprintf("- Current directory: %s\n", currentDir)
	}

	// 列出当前目录中的文件和目录
	files, err := utils.ListFiles(".")
	if err == nil && len(files) > 0 {
		envInfo += "- Files and directories in current directory:\n"
		for _, file := range files {
			envInfo += fmt.Sprintf("  - %s\n", file)
		}
	}

	// 创建本地化器
	localizer := i18n.NewLocalizer(bundle, cfg.Language)

	// 使用翻译
	return localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "generateScriptPrompt",
		TemplateData: map[string]interface{}{
			"Shell":  shell,
			"OS":     os,
			"Prompt": prompt + envInfo,
		},
	})
}
