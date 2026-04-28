package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	xterm "golang.org/x/term"

	"github.com/AlecAivazis/survey/v2"
	"github.com/joho/godotenv"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github/xiuivfbc/NaturalCmd/internal/completion"
	"github/xiuivfbc/NaturalCmd/internal/config"
	"github/xiuivfbc/NaturalCmd/internal/executor"
	"github/xiuivfbc/NaturalCmd/internal/history"
	"github/xiuivfbc/NaturalCmd/internal/rag"
	"github/xiuivfbc/NaturalCmd/internal/skills"
	"github/xiuivfbc/NaturalCmd/internal/ui"
	"github/xiuivfbc/NaturalCmd/internal/utils"
)

const maxAutoFormatRetries = 3

const longModeToggleCommand = "-l"

type conversationTurn struct {
	Prompt string // 用户输入或命令
	Script string // 生成的脚本或执行结果
	Type   string // "llm" 或 "shell"，标识来源
}

func recordConversationTurn(history *[]conversationTurn, longMode bool, prompt, script, turnType string) {
	if !longMode || strings.TrimSpace(prompt) == "" || strings.TrimSpace(script) == "" {
		return
	}

	*history = append(*history, conversationTurn{
		Prompt: prompt,
		Script: script,
		Type:   turnType,
	})
}

// normalizeExclamation 将中文感叹号 ！ 替换为英文感叹号 !
func normalizeExclamation(s string) string {
	return strings.ReplaceAll(s, "！", "!")
}

// 全局 i18n bundle
var bundle *i18n.Bundle

func main() {
	if err := godotenv.Load(); err != nil {
		// 创建默认本地化器（英文）
		localizer := i18n.NewLocalizer(bundle, "en")
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "envFileNotFound",
		}))
	}
	var prompt string
	var historyQuery string
	var silent bool
	var longMode bool

	flag.StringVar(&prompt, "p", "", "Prompt to run")
	flag.StringVar(&historyQuery, "h", "", "Search prompt history")
	flag.BoolVar(&silent, "s", false, "Skip explanation generation")
	flag.BoolVar(&longMode, "l", false, "Enable long conversation mode")
	normalizedArgs, historyFlagUsed := normalizeArgs(os.Args[1:])
	if err := flag.CommandLine.Parse(normalizedArgs); err != nil {
		os.Exit(2)
	}
	historyRequested := historyFlagUsed && strings.TrimSpace(historyQuery) == ""

	// 如果命令行参数中没有指定prompt，从剩余参数中获取
	if prompt == "" {
		prompt = strings.Join(flag.Args(), " ")
	}

	// 加载配置
	cfg, localizer, err := loadConfig()
	if err != nil {
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "errorLoadingConfig",
			TemplateData: map[string]interface{}{
				"Error": err,
			},
		}))
		os.Exit(1)
	}

	// 设置 signal 处理程序以捕获 Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println()
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "goodbyeMessage",
		}))
		os.Exit(0)
	}()

	var skillRegistry *skills.Registry
	if cfg.SkillsEnabled {
		skillRegistry, err = skills.Load(cfg.SkillsFile)
		if err != nil {
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "errorLoadingSkills",
				TemplateData: map[string]interface{}{
					"Error": err,
				},
			}))
			skillRegistry = &skills.Registry{}
		}
	}

	// 检查API密钥是否设置。若存在本地 skill，则允许以 skill-only 模式运行。
	if !checkAPIKey(cfg, localizer, skillRegistry) {
		os.Exit(1)
	}

	historyStore, err := history.Load(cfg.HistoryFile, cfg.HistoryMax)
	if err != nil {
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "errorLoadingHistory",
			TemplateData: map[string]interface{}{
				"Error": err,
			},
		}))
	}

	var feedbackStore *rag.FeedbackStore
	if cfg.RAGEnabled {
		feedbackStore, err = rag.LoadFeedback(cfg.RAGFeedbackFile)
		if err != nil {
			fmt.Printf("Warning: failed to load rag feedback store: %v\n", err)
		}
	}

	// 历史记录查询和选择
	if historyStore != nil && (historyRequested || historyQuery != "") {
		resolvedPrompt, selectedFromHistory, shouldExit := promptFromHistory(historyQuery, historyStore, localizer)
		if shouldExit {
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "goodbyeMessage",
			}))
			os.Exit(0)
		}
		if selectedFromHistory {
			prompt = resolvedPrompt
		}
	}

	if longMode {
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "longModeEnabled",
		}))
	}

	conversationHistory := make([]conversationTurn, 0, 8)
	var shellDirectMode bool

	for {
		// 获取用户输入（如果没有指定prompt）
		if prompt == "" {
			prompt = getUserPrompt(localizer, historyStore)
			if prompt == "" {
				continue
			}
		}

		if strings.TrimSpace(prompt) == longModeToggleCommand {
			longMode = !longMode
			if longMode {
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "longModeEnabled",
				}))
			} else {
				conversationHistory = conversationHistory[:0]
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "longModeDisabled",
				}))
			}
			prompt = ""
			continue
		}

		// 检测直连命令模式切换（单独的 !）
		trimmedPrompt := strings.TrimSpace(prompt)
		trimmedPrompt = normalizeExclamation(trimmedPrompt)
		if trimmedPrompt == "!" {
			shellDirectMode = !shellDirectMode
			if shellDirectMode {
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "shellDirectModeEnabled",
				}))
			} else {
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "shellDirectModeDisabled",
				}))
			}
			prompt = ""
			continue
		}

		// 在直连模式中执行所有输入
		if shellDirectMode {
			if trimmedPrompt != "" {
				clearScreenIfSupported()
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "executingCommand",
				}))
				fmt.Printf("Running: %s\n", trimmedPrompt)
				result, err := executor.ExecuteCommand(trimmedPrompt)

				// 收集执行结果
				var executionResult string
				if err != nil {
					fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
						MessageID: "errorExecutingCommand",
						TemplateData: map[string]interface{}{
							"Error": err,
						},
					}))
					executionResult = fmt.Sprintf("Error: %v", err)
				} else {
					executionResult = "Success"
					if result != nil {
						if result.Stdout != "" {
							executionResult += "\n" + result.Stdout
						}
						if result.Stderr != "" {
							executionResult += "\nStderr: " + result.Stderr
						}
						if result.ExitCode != 0 {
							fmt.Printf("Exit code: %d\n", result.ExitCode)
							executionResult += fmt.Sprintf("\nExit code: %d", result.ExitCode)
						}
					}
				}

				// 如果在长对话模式，记录到历史
				if longMode {
					recordConversationTurn(&conversationHistory, longMode, trimmedPrompt, executionResult, "shell")
				}
			}
			prompt = ""
			continue
		}

		// 直连命令（非持久模式，!cmd 形式在 LLM 模式下仍可用）
		if strings.HasPrefix(trimmedPrompt, "!") {
			command := strings.TrimSpace(strings.TrimPrefix(trimmedPrompt, "!"))
			if command != "" {
				clearScreenIfSupported()
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "executingCommand",
				}))
				fmt.Printf("Running: %s\n", command)
				result, err := executor.ExecuteCommand(command)

				// 收集执行结果
				var executionResult string
				if err != nil {
					fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
						MessageID: "errorExecutingCommand",
						TemplateData: map[string]interface{}{
							"Error": err,
						},
					}))
					executionResult = fmt.Sprintf("Error: %v", err)
				} else {
					executionResult = "Success"
					if result != nil {
						if result.Stdout != "" {
							executionResult += "\n" + result.Stdout
						}
						if result.Stderr != "" {
							executionResult += "\nStderr: " + result.Stderr
						}
						if result.ExitCode != 0 {
							fmt.Printf("Exit code: %d\n", result.ExitCode)
							executionResult += fmt.Sprintf("\nExit code: %d", result.ExitCode)
						}
					}
				}

				// 如果在长对话模式，记录到历史
				if longMode {
					recordConversationTurn(&conversationHistory, longMode, command, executionResult, "shell")
				}

				prompt = ""
				continue
			}
		}

		initialPrompt := strings.TrimSpace(prompt)
		executionFeedback := ""
		formatRetryCount := 0

		for {
			conversationContext := ""
			if longMode {
				conversationContext = buildConversationContext(conversationHistory)
			}

			// 生成脚本和解释
			script, err := generateScriptAndExplanation(prompt, conversationContext, executionFeedback, cfg, localizer, historyStore, feedbackStore, skillRegistry, silent, longMode)
			if err != nil {
				var formatErr *utils.ScriptFormatError
				if errors.As(err, &formatErr) {
					formatRetryCount++
					if formatRetryCount > maxAutoFormatRetries {
						fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
							MessageID: "errorGeneratingScript",
							TemplateData: map[string]interface{}{
								"Error": err,
							},
						}))
						os.Exit(1)
					}
					executionFeedback = buildFormatFeedback(formatErr)
					continue
				}
				os.Exit(1)
			}
			formatRetryCount = 0

			// 让用户选择是否执行
			selectedOption := promptUserForAction(localizer)
			if selectedOption == "" {
				continue
			}

			if selectedOption == "confirm" {
				// 执行命令
				result, err := executeCommand(script, localizer)
				if err != nil {
					if feedbackStore != nil {
						_ = feedbackStore.RecordFailure(script)
					}
					fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
						MessageID: "errorExecutingCommand",
						TemplateData: map[string]interface{}{
							"Error": err,
						},
					}))
					fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
						MessageID: "retryingAfterExecutionError",
					}))
					executionFeedback = buildExecutionFeedback(script, result, err)
					continue
				}
				if historyStore != nil {
					if err := historyStore.Add(initialPrompt, script); err != nil {
						fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
							MessageID: "errorSavingHistory",
							TemplateData: map[string]interface{}{
								"Error": err,
							},
						}))
					}
				}
				if feedbackStore != nil {
					_ = feedbackStore.RecordSuccess(script)
				}
				if longMode {
					recordConversationTurn(&conversationHistory, longMode, strings.TrimSpace(prompt), strings.TrimSpace(script), "llm")
				}
				printSuccessCelebration(localizer, initialPrompt, script)
				prompt = ""
				break
			} else if selectedOption == "cancel" {
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "goodbyeMessage",
				}))
				os.Exit(0)
			} else if selectedOption == "retry" {
				// 让用户输入补充词
				var additionalInfo string
				for additionalInfo == "" {
					value, err := ui.GetAdditionalInfo(localizer)
					if err != nil {
						if ui.IsInterrupt(err) {
							fmt.Println()
							fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
								MessageID: "goodbyeMessage",
							}))
							os.Exit(0)
						}
						fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
							MessageID: "errorReadingInput",
							TemplateData: map[string]interface{}{
								"Error": err,
							},
						}))
						continue
					}
					additionalInfo = value
				}
				// 更新prompt，添加补充词
				prompt = prompt + " " + additionalInfo
				executionFeedback = ""
				formatRetryCount = 0
				continue
			}
		}
	}
}

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

// 加载配置并返回配置和本地化器
func loadConfig() (*config.Config, *i18n.Localizer, error) {
	cfg, err := config.Load()
	if err != nil {
		// 创建默认本地化器（英文）
		localizer := i18n.NewLocalizer(bundle, "en")
		return nil, localizer, err
	}

	// 创建本地化器
	localizer := i18n.NewLocalizer(bundle, cfg.Language)
	return cfg, localizer, nil
}

// 检查API密钥是否设置
func checkAPIKey(cfg *config.Config, localizer *i18n.Localizer, skillRegistry *skills.Registry) bool {
	if cfg.APIKey == "" {
		if skillRegistry != nil && skillRegistry.HasSkills() {
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "apiKeyMissingSkillOnlyMode",
			}))
			return true
		}

		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "errorApiKeyNotSet",
		}))
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "setApiKeyHint",
		}))
		return false
	}
	return true
}

// 获取用户输入的prompt
func getUserPrompt(localizer *i18n.Localizer, historyStore *history.Store) string {
	prompt := ""
	for prompt == "" {
		value, err := ui.GetPrompt(localizer)
		if err != nil {
			if ui.IsInterrupt(err) {
				fmt.Println()
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "goodbyeMessage",
				}))
				os.Exit(0)
			}
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "errorReadingInput",
				TemplateData: map[string]interface{}{
					"Error": err,
				},
			}))
			continue
		}

		if historyStore != nil {
			if query, ok := parseInlineHistoryQuery(value); ok {
				resolvedPrompt, selectedFromHistory, shouldExit := promptFromHistory(query, historyStore, localizer)
				if shouldExit {
					continue
				}
				if selectedFromHistory {
					prompt = resolvedPrompt
				}
				continue
			}
		}

		if strings.TrimSpace(value) == "" {
			clearScreenIfSupported()
			continue
		}

		prompt = value
	}
	return prompt
}

func parseInlineHistoryQuery(input string) (string, bool) {
	value := strings.TrimSpace(input)
	if value == "-h" {
		return "", true
	}

	if strings.HasPrefix(value, "-h ") {
		query := strings.TrimSpace(strings.TrimPrefix(value, "-h "))
		query = strings.Trim(query, "\"'")
		return query, true
	}

	return "", false
}

// 生成脚本和解释
func generateScriptAndExplanation(prompt string, conversationContext string, executionFeedback string, cfg *config.Config, localizer *i18n.Localizer, historyStore *history.Store, feedbackStore *rag.FeedbackStore, skillRegistry *skills.Registry, silent bool, debugLongMode bool) (string, error) {
	promptForModel := strings.TrimSpace(prompt)
	if strings.TrimSpace(conversationContext) != "" {
		promptForModel += "\n\n" + conversationContext
	}
	if strings.TrimSpace(executionFeedback) == "" && skillRegistry != nil && skillRegistry.HasSkills() {
		if cfg.APIKey != "" {
			selection, err := completion.GenerateSkillSelection(prompt, skillRegistry.BuildCatalog(), cfg, debugLongMode)
			if err != nil {
				fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "smartSkillSelectFailed",
					TemplateData: map[string]interface{}{
						"Error": err,
					},
				}))
			} else if len(selection.SelectedSkills) > 0 {
				selectedContext := skillRegistry.BuildSelectedContext(selection.SelectedSkills, runtime.GOOS)
				if strings.TrimSpace(selectedContext) != "" {
					fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
						MessageID: "smartSkillSelected",
						TemplateData: map[string]interface{}{
							"Skills": strings.Join(selection.SelectedSkills, ", "),
						},
					}))
					if strings.TrimSpace(selection.Strategy) != "" {
						fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
							MessageID: "smartSkillStrategy",
							TemplateData: map[string]interface{}{
								"Strategy": selection.Strategy,
							},
						}))
					}
					promptForModel += "\n\n" + selectedContext
				}
			}
		} else if resolved, ok := skillRegistry.ResolveCurrentOS(prompt); ok {
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "matchedSkill",
				TemplateData: map[string]interface{}{
					"Name": resolved.Skill.Name,
				},
			}))
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "matchedSkillTrigger",
				TemplateData: map[string]interface{}{
					"Trigger": resolved.MatchedBy,
				},
			}))

			fmt.Printf("\n%s\n", localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "generatedScript",
				TemplateData: map[string]interface{}{
					"Script": resolved.Script,
				},
			}))

			if !silent && !cfg.SilentMode {
				fmt.Printf("\n%s\n", localizer.MustLocalize(&i18n.LocalizeConfig{
					MessageID: "explanation",
				}))

				description := strings.TrimSpace(resolved.Skill.Description)
				if description == "" {
					description = localizer.MustLocalize(&i18n.LocalizeConfig{
						MessageID: "matchedSkillDefaultDescription",
						TemplateData: map[string]interface{}{
							"Count": len(resolved.Skill.Commands),
						},
					})
				}
				fmt.Println(description)
			}

			return resolved.Script, nil
		}
	}

	if cfg.APIKey == "" {
		err := errors.New(localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "apiKeyRequiredForModelGeneration"}))
		fmt.Println(err.Error())
		return "", err
	}

	if cfg.RAGEnabled {
		match := rag.BuildHistoryMatchWithFeedback(prompt, historyStore, feedbackStore, cfg.RAGTopK)
		ragContext := match.Context
		localHit := match.BestScore >= cfg.RAGMinLocalHit && match.Coverage >= cfg.RAGMinLocalCover
		if !localHit && cfg.RAGSemanticExpand {
			expandedTerms, err := completion.GenerateQueryExpansion(prompt, cfg, debugLongMode)
			if err == nil && strings.TrimSpace(expandedTerms) != "" {
				expandedMatch := rag.BuildHistoryMatchWithFeedback(prompt+" "+expandedTerms, historyStore, feedbackStore, cfg.RAGTopK)
				if expandedMatch.BestScore >= cfg.RAGMinLocalHit && expandedMatch.Coverage >= cfg.RAGMinLocalCover {
					ragContext = expandedMatch.Context
				} else {
					ragContext = ""
				}
			} else {
				ragContext = ""
			}
		}
		if ragContext != "" {
			promptForModel += "\n\n" + ragContext
		}
	}

	if strings.TrimSpace(executionFeedback) != "" {
		promptForModel += executionFeedback
	}

	// 生成命令（流式输出在 completion 层处理）
	script, err := completion.GenerateScript(promptForModel, cfg, debugLongMode)
	if err != nil {
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "errorGeneratingScript",
			TemplateData: map[string]interface{}{
				"Error": err,
			},
		}))
		return "", err
	}

	if err := utils.ValidateScriptOutput(script); err != nil {
		return "", err
	}

	fmt.Printf("\n%s\n", localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "generatedScript",
		TemplateData: map[string]interface{}{
			"Script": script,
		},
	}))

	// 生成解释（如果非静默模式）
	if !silent && !cfg.SilentMode {
		fmt.Printf("\n%s\n", localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "explanation",
		}))
		_, err := completion.GenerateExplanation(script, cfg, debugLongMode)
		if err != nil {
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "errorGeneratingExplanation",
				TemplateData: map[string]interface{}{
					"Error": err,
				},
			}))
		}
	}

	return script, nil
}

// 提示用户选择操作
func promptUserForAction(localizer *i18n.Localizer) string {
	var selectedOption string
	confirmOption := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "executeScriptOptionConfirm",
	})
	retryOption := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "executeScriptOptionRetry",
	})
	cancelOption := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "executeScriptOptionCancel",
	})
	selectPrompt := &survey.Select{
		Message: localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "executeScriptQuestion",
		}),
		Options: []string{confirmOption, retryOption, cancelOption},
		Default: confirmOption,
	}

	err := survey.AskOne(selectPrompt, &selectedOption)
	if err != nil {
		if ui.IsInterrupt(err) {
			fmt.Println()
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "goodbyeMessage",
			}))
			os.Exit(0)
		}
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "errorReadingInput",
			TemplateData: map[string]interface{}{
				"Error": err,
			},
		}))
		return ""
	}

	if selectedOption == confirmOption {
		return "confirm"
	}
	if selectedOption == retryOption {
		return "retry"
	}
	if selectedOption == cancelOption {
		return "cancel"
	}

	return ""
}

// 执行命令
func executeCommand(script string, localizer *i18n.Localizer) (*executor.ExecutionResult, error) {
	clearScreenIfSupported()
	fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "executingCommand",
	}))
	fmt.Printf("Running: %s\n", script)
	result, err := executor.ExecuteCommand(script)
	if err != nil {
		return result, err
	}
	return result, nil
}

func clearScreenIfSupported() {
	// Only attempt to clear when stdout is a terminal.
	if !xterm.IsTerminal(int(os.Stdout.Fd())) {
		return
	}

	// 直接向当前终端输出清屏序列，确保影响的是当前会话而不是子进程。
	// Windows Terminal / VS Code 终端 / 大多数现代终端都支持这一方式。
	fmt.Print("\033[H\033[2J")
}

func printSuccessCelebration(localizer *i18n.Localizer, initialPrompt string, finalScript string) {
	title := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "celebrationTitle",
	})
	promptLabel := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "celebrationInitialPromptLabel",
	})
	scriptLabel := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "celebrationFinalScriptLabel",
	})

	line := strings.Repeat("=", 72)
	fmt.Println()
	fmt.Println(line)
	fmt.Printf("= %s\n", title)
	fmt.Printf("= %s %s\n", promptLabel, strings.TrimSpace(initialPrompt))
	fmt.Printf("= %s %s\n", scriptLabel, strings.TrimSpace(finalScript))
	fmt.Println(line)
	fmt.Println()
}

func buildExecutionFeedback(script string, result *executor.ExecutionResult, err error) string {
	var builder strings.Builder

	builder.WriteString("\n\nThe previous generated command failed. Analyze the failure and generate a corrected replacement command.\n")
	builder.WriteString("Failed command:\n")
	builder.WriteString(script)
	builder.WriteString("\n")
	builder.WriteString("Execution error:\n")
	builder.WriteString(err.Error())
	builder.WriteString("\n")

	if result != nil {
		if result.ExitCode != 0 {
			builder.WriteString("Exit code:\n")
			builder.WriteString(strconv.Itoa(result.ExitCode))
			builder.WriteString("\n")
		}

		stdout := trimExecutionOutput(result.Stdout)
		if stdout != "" {
			builder.WriteString("Captured stdout:\n")
			builder.WriteString(stdout)
			builder.WriteString("\n")
		}

		stderr := trimExecutionOutput(result.Stderr)
		if stderr != "" {
			builder.WriteString("Captured stderr:\n")
			builder.WriteString(stderr)
			builder.WriteString("\n")
		}
	}

	builder.WriteString("Return a new single-line command only. Do not explain it.\n")
	return builder.String()
}

func buildFormatFeedback(formatErr *utils.ScriptFormatError) string {
	var builder strings.Builder

	builder.WriteString("\n\nThe previous model output was invalid because it did not match the required single-line command format.\n")
	if strings.TrimSpace(formatErr.Reason) != "" {
		builder.WriteString("Validation error:\n")
		builder.WriteString(formatErr.Reason)
		builder.WriteString("\n")
	}

	if trimmed := trimExecutionOutput(formatErr.Script); trimmed != "" {
		builder.WriteString("Invalid output:\n")
		builder.WriteString(trimmed)
		builder.WriteString("\n")
	}

	builder.WriteString("Return exactly one single-line executable command only. No markdown, no explanation, no code fences, no shell launcher.\n")
	return builder.String()
}

func buildConversationContext(history []conversationTurn) string {
	if len(history) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Long conversation context (keep consistency with these prior turns):\n")
	for index, turn := range history {
		prompt := strings.TrimSpace(turn.Prompt)
		script := strings.TrimSpace(turn.Script)
		if prompt == "" || script == "" {
			continue
		}

		if turn.Type == "shell" {
			builder.WriteString(fmt.Sprintf("%d. [Shell Command] %s\n", index+1, prompt))
			builder.WriteString(fmt.Sprintf("   Result: %s\n", script))
		} else {
			// 默认为 "llm" 类型
			builder.WriteString(fmt.Sprintf("%d. User request: %s\n", index+1, prompt))
			builder.WriteString(fmt.Sprintf("   Final command: %s\n", script))
		}
	}
	builder.WriteString("Generate the next command with this context in mind.\n")

	return strings.TrimSpace(builder.String())
}

func promptFromHistory(query string, historyStore *history.Store, localizer *i18n.Localizer) (string, bool, bool) {
	entries := historyStore.Search(query)
	if len(entries) == 0 {
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "historyNoMatches",
			TemplateData: map[string]interface{}{
				"Query": query,
			},
		}))
		return "", false, true
	}

	noneOption := localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "historyNoneOption",
	})

	options := make([]string, 0, len(entries)+1)
	selectedPromptByOption := make(map[string]string, len(entries))
	for _, entry := range entries {
		option := formatHistoryOption(entry)
		options = append(options, option)
		selectedPromptByOption[option] = entry.Prompt
	}
	options = append(options, noneOption)

	selectedOption, err := ui.SelectOption(localizer.MustLocalize(&i18n.LocalizeConfig{
		MessageID: "historySelectPrompt",
	}), options)
	if err != nil {
		if ui.IsInterrupt(err) {
			fmt.Println()
			fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
				MessageID: "goodbyeMessage",
			}))
			os.Exit(0)
		}
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "errorReadingInput",
			TemplateData: map[string]interface{}{
				"Error": err,
			},
		}))
		return "", false, false
	}

	if selectedOption == noneOption {
		return "", false, false
	}

	return selectedPromptByOption[selectedOption], true, false
}

func normalizeArgs(args []string) ([]string, bool) {
	normalized := make([]string, 0, len(args)+2)
	historyFlagUsed := false

	for index := 0; index < len(args); index++ {
		arg := args[index]

		switch arg {
		case "-hs", "-sh":
			normalized = append(normalized, "-s")
			historyFlagUsed = true
			if index == len(args)-1 || strings.HasPrefix(args[index+1], "-") {
				normalized = append(normalized, "-h=")
			} else {
				normalized = append(normalized, "-h")
			}
			continue
		case "-ps", "-sp":
			// -p 需要后续参数，这里把 -s 提前展开，避免被当成 -p 的值
			normalized = append(normalized, "-s", "-p")
			continue
		case "-h":
			historyFlagUsed = true
			if index == len(args)-1 || strings.HasPrefix(args[index+1], "-") {
				normalized = append(normalized, "-h=")
				continue
			}
		}

		if strings.HasPrefix(arg, "-h=") {
			historyFlagUsed = true
		}

		normalized = append(normalized, arg)
	}

	return normalized, historyFlagUsed
}

func formatHistoryOption(entry history.Entry) string {
	prompt := truncateForOption(entry.Prompt, 48)
	script := truncateForOption(entry.Script, 36)
	if script == "" {
		return prompt
	}

	return fmt.Sprintf("%s => %s", prompt, script)
}

func truncateForOption(value string, maxLen int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}

func trimExecutionOutput(output string) string {
	const maxOutputLength = 4000

	output = strings.TrimSpace(output)
	if len(output) <= maxOutputLength {
		return output
	}

	const retainedLength = maxOutputLength / 2
	return output[:retainedLength] + "\n...\n" + output[len(output)-retainedLength:]
}
