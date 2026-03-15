package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/joho/godotenv"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github/xiuivfbc/NaturalCmd/internal/completion"
	"github/xiuivfbc/NaturalCmd/internal/config"
	"github/xiuivfbc/NaturalCmd/internal/executor"
	"github/xiuivfbc/NaturalCmd/internal/history"
	"github/xiuivfbc/NaturalCmd/internal/rag"
	"github/xiuivfbc/NaturalCmd/internal/ui"
)

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

	flag.StringVar(&prompt, "p", "", "Prompt to run")
	flag.StringVar(&historyQuery, "h", "", "Search prompt history")
	flag.BoolVar(&silent, "s", false, "Skip explanation generation")
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

	// 检查API密钥是否设置
	if !checkAPIKey(cfg, localizer) {
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

	for {
		// 获取用户输入（如果没有指定prompt）
		if prompt == "" {
			prompt = getUserPrompt(localizer, historyStore)
			if prompt == "" {
				continue
			}
		}

		initialPrompt := strings.TrimSpace(prompt)
		executionFeedback := ""

		for {
			// 生成脚本和解释
			script, err := generateScriptAndExplanation(prompt, executionFeedback, cfg, localizer, historyStore, feedbackStore, silent)
			if err != nil {
				os.Exit(1)
			}

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
func checkAPIKey(cfg *config.Config, localizer *i18n.Localizer) bool {
	if cfg.APIKey == "" {
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
func generateScriptAndExplanation(prompt string, executionFeedback string, cfg *config.Config, localizer *i18n.Localizer, historyStore *history.Store, feedbackStore *rag.FeedbackStore, silent bool) (string, error) {
	promptForModel := strings.TrimSpace(prompt)
	if cfg.RAGEnabled {
		ragContext := rag.BuildHistoryContextWithFeedback(prompt, historyStore, feedbackStore, cfg.RAGTopK)
		if ragContext == "" && cfg.RAGSemanticExpand {
			expandedTerms, err := completion.GenerateQueryExpansion(prompt, cfg)
			if err == nil && strings.TrimSpace(expandedTerms) != "" {
				ragContext = rag.BuildHistoryContextWithFeedback(prompt+" "+expandedTerms, historyStore, feedbackStore, cfg.RAGTopK)
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
	script, err := completion.GenerateScript(promptForModel, cfg)
	if err != nil {
		fmt.Println(localizer.MustLocalize(&i18n.LocalizeConfig{
			MessageID: "errorGeneratingScript",
			TemplateData: map[string]interface{}{
				"Error": err,
			},
		}))
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
		_, err := completion.GenerateExplanation(script, cfg)
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

	fmt.Println()
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
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" || strings.EqualFold(term, "dumb") {
		return
	}

	// ANSI clear screen + move cursor to top-left, similar to Ctrl+L in common terminals.
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
