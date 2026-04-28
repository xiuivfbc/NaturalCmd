package executor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ExecutionResult 包含命令执行结果
type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ExecuteCommand 执行命令
func ExecuteCommand(script string) (*ExecutionResult, error) {
	if err := validateCommand(script); err != nil {
		return &ExecutionResult{
			Stderr:   err.Error(),
			ExitCode: -1,
		}, err
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// 在 Windows 上切换代码页为 UTF-8 (65001) 以支持中文输出
		script = "chcp 65001 > nul && " + script
		cmd = exec.Command("cmd.exe", "/c", script)
	} else {
		cmd = exec.Command("sh", "-c", script)
	}

	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer

	// 在 Windows 上只缓冲输出，不实时输出到 os.Stdout（避免 GBK 编码问题）
	// 在 Unix 上实时输出以保持交互性
	if runtime.GOOS == "windows" {
		cmd.Stdout = &stdoutBuffer
		cmd.Stderr = &stderrBuffer
	} else {
		cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuffer)
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuffer)
	}
	cmd.Stdin = os.Stdin

	err := cmd.Run()

	// 在 Windows 上将 GBK 编码输出转换为 UTF-8
	stdoutStr := stdoutBuffer.String()
	stderrStr := stderrBuffer.String()
	if runtime.GOOS == "windows" {
		stdoutStr = decodeGBK(stdoutStr)
		stderrStr = decodeGBK(stderrStr)
		// 转换完成后输出到终端
		if stdoutStr != "" {
			fmt.Print(stdoutStr)
		}
		if stderrStr != "" {
			fmt.Fprint(os.Stderr, stderrStr)
		}
	}

	result := &ExecutionResult{
		Stdout:   stdoutStr,
		Stderr:   stderrStr,
		ExitCode: exitCode(cmd, err),
	}

	return result, err
}

func validateCommand(script string) error {
	fields := strings.Fields(strings.TrimSpace(script))
	if len(fields) == 0 {
		return fmt.Errorf("empty command is not allowed")
	}

	command := normalizeCommandName(fields[0])
	if isForbiddenShellLauncher(command) {
		return fmt.Errorf("shell launcher commands are not allowed: %s", fields[0])
	}

	return nil
}

func normalizeCommandName(command string) string {
	command = strings.Trim(command, "\"'")
	command = strings.ReplaceAll(command, "\\", "/")
	return strings.ToLower(filepath.Base(command))
}

func isForbiddenShellLauncher(command string) bool {
	switch command {
	case "bash", "sh", "zsh", "fish", "dash", "ksh", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return true
	default:
		return false
	}
}

func decodeGBK(s string) string {
	if s == "" {
		return s
	}

	by := []byte(s)
	I := bytes.NewReader(by)
	O := transform.NewReader(I, simplifiedchinese.GBK.NewDecoder())
	out, err := io.ReadAll(O)
	if err != nil {
		// 转换失败时返回原字符串
		return s
	}
	return string(out)
}

func exitCode(cmd *exec.Cmd, err error) int {
	if err == nil {
		return 0
	}

	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}

	return -1
}
