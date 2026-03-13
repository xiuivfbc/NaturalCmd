package utils

import (
	"os"
	"runtime"
	"strings"
)

// GetShell 获取当前shell
func GetShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			return "cmd"
		}
		return "bash"
	}
	return shell
}

// GetOS 获取当前操作系统
func GetOS() string {
	return runtime.GOOS
}

// ExtractScript 从响应中提取脚本，移除代码块标记
func ExtractScript(response string) string {
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSpace(response)
		if strings.Contains(response, "\n") {
			lines := strings.Split(response, "\n")
			firstLine := strings.TrimSpace(lines[0])
			// 第一行是语言标识（如 bash/sh/zsh），跳过它取真正的命令行
			if isShellTag(firstLine) {
				for _, line := range lines[1:] {
					line = strings.TrimSpace(line)
					if line != "" && line != "```" {
						response = line
						break
					}
				}
			} else {
				response = firstLine
			}
		}
	}
	// 移除末尾的代码块标记
	if strings.HasSuffix(response, "```") {
		response = strings.TrimSuffix(response, "```")
	}
	// 移除残余反引号
	response = strings.TrimPrefix(response, "`")
	response = strings.TrimSuffix(response, "`")
	return strings.TrimSpace(response)
}

// isShellTag 判断字符串是否是 shell 语言标识符
func isShellTag(s string) bool {
	switch strings.ToLower(s) {
	case "bash", "sh", "zsh", "fish", "cmd", "powershell", "pwsh", "shell":
		return true
	}
	return false
}

// ListFiles 列出指定目录中的文件和目录
func ListFiles(dir string) ([]string, error) {
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			files = append(files, "[DIR] "+entry.Name())
		} else {
			files = append(files, "[FILE] "+entry.Name())
		}
	}
	return files, nil
}

// FileExists 检查文件是否存在
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// GetCurrentDir 获取当前工作目录
func GetCurrentDir() (string, error) {
	return os.Getwd()
}

// GetFileContent 读取文件内容
func GetFileContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
