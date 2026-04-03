package utils

import (
	"os"
	"runtime"
	"strings"
	"unicode"
)

// ScriptFormatError 表示模型返回结果不符合单行命令格式要求。
type ScriptFormatError struct {
	Reason string
	Script string
}

func (e *ScriptFormatError) Error() string {
	return e.Reason
}

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

// ValidateScriptOutput 检查模型输出是否符合可执行单行命令的基本格式。
func ValidateScriptOutput(script string) error {
	value := strings.TrimSpace(script)
	if value == "" {
		return &ScriptFormatError{Reason: "empty response", Script: script}
	}

	if strings.ContainsAny(value, "\r\n") {
		return &ScriptFormatError{Reason: "response must be a single line command", Script: script}
	}

	if strings.Contains(value, "```") {
		return &ScriptFormatError{Reason: "markdown code fences are not allowed", Script: script}
	}

	fields := strings.Fields(value)
	if len(fields) == 0 {
		return &ScriptFormatError{Reason: "empty response", Script: script}
	}

	firstToken := strings.TrimLeft(strings.Trim(fields[0], "\"'"), "@")
	if firstToken == "" || !looksLikeCommandToken(firstToken) {
		return &ScriptFormatError{Reason: "response does not start with a command-like token", Script: script}
	}

	commandName := normalizeCommandName(firstToken)
	if isForbiddenShellLauncher(commandName) {
		return &ScriptFormatError{Reason: "shell launcher commands are not allowed", Script: script}
	}

	return nil
}

// isShellTag 判断字符串是否是 shell 语言标识符
func isShellTag(s string) bool {
	switch strings.ToLower(s) {
	case "bash", "sh", "zsh", "fish", "cmd", "powershell", "pwsh", "shell":
		return true
	}
	return false
}

func looksLikeCommandToken(token string) bool {
	if token == "" {
		return false
	}

	first := []rune(token)[0]
	if first > unicode.MaxASCII {
		return false
	}

	if unicode.IsLetter(first) || unicode.IsDigit(first) {
		return true
	}

	switch first {
	case '.', '/', '\\', '~', '$', ':':
		return true
	default:
		return false
	}
}

func normalizeCommandName(command string) string {
	command = strings.Trim(command, "\"'")
	command = strings.ReplaceAll(command, "\\", "/")
	return strings.ToLower(strings.TrimPrefix(filepathBase(command), "@"))
}

func isForbiddenShellLauncher(command string) bool {
	switch command {
	case "bash", "sh", "zsh", "fish", "dash", "ksh", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return true
	default:
		return false
	}
}

func filepathBase(path string) string {
	lastSlash := strings.LastIndexAny(path, "/\\")
	if lastSlash >= 0 {
		return path[lastSlash+1:]
	}
	return path
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
