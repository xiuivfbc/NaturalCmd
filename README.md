# NaturalCmd

一个用 Go 实现的 Cmd，将自然语言转换为 Cmd 命令的命令行工具。

## 功能

- 将自然语言转换为 Cmd 命令
- 自动执行生成的命令
- 为生成的命令提供解释
- 支持智能 Skill：AI 根据提示词自主选择并组合技能
- 流式响应，提供更好的用户体验
- 环境感知能力，能够获取当前目录文件状态
- 智能错误处理，执行失败时会自动重新生成脚本
- 支持多语言输出（中文/英文）
- 支持不同的 AI 模型提供商（OpenAI、阿里云）

## 示例
亲测有效：![alt text](image.png)

## 安装

### 前提条件

- Go 1.20 或更高版本
- AI 模型 API 密钥（OpenAI 或阿里云）

### 构建

```bash
# Windows
go build -o naturalcmd.exe ./cmd/main.go

# Linux/macOS
go build -o naturalcmd ./cmd/main.go
```

## 使用

### 双模型配置（推荐）- 降低成本 40-50%

NaturalCmd 支持 **主副模型分离策略**，实现成本和性能的平衡：

| 模型用途 | 适用场景 | 复杂度 | 推荐选择 |
|---------|---------|--------|---------|
| **主模型** | 脚本生成（核心功能） | ⭐⭐⭐⭐⭐ | GPT-4o、Claude-3.5 |
| **副模型** | 脚本解释、查询扩展、技能选择 | ⭐⭐ | GPT-4o-mini、Claude-3-haiku |

**特点：**
- ✅ 两个模型可来自不同厂商（如 OpenAI 主模型 + 阿里云副模型）
- ✅ 独立配置 API Key、Endpoint
- ✅ 完全向后兼容（不配置时自动使用单模型）
- ✅ 成本节省 40-50% 而不影响核心体验

**跨厂商配置示例（OpenAI + 阿里云）：**

```env
# 主模型：OpenAI GPT-4o（脚本生成）
MODEL_PRIMARY=gpt-4o
MODEL_PRIMARY_PROVIDER=openai
MODEL_PRIMARY_KEY=sk-xxx-your-openai-key
MODEL_PRIMARY_ENDPOINT=https://api.openai.com/v1/chat/completions

# 副模型：阿里云 Qwen（解释、扩展、技能选择）
MODEL_SECONDARY=qwen2.5-7b-instruct
MODEL_SECONDARY_PROVIDER=aliyun
MODEL_SECONDARY_KEY=sk-xxx-your-aliyun-key
MODEL_SECONDARY_ENDPOINT=https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions
```

**同厂商简化配置示例：**

```env
# 主模型
MODEL_PRIMARY=gpt-4o
MODEL_PRIMARY_PROVIDER=openai
MODEL_PRIMARY_KEY=sk-xxx

# 副模型（如未指定以下字段，自动使用主模型配置）
MODEL_SECONDARY=gpt-4o-mini
```

完整配置选项见 [.env.example](.env.example)

### 设置 API 密钥

#### 使用 .env 文件（旧式单模型配置）

在项目根目录下创建一个 `.env` 文件，添加以下内容：

```env
# API 密钥
API_KEY=your-aliyun-api-key

# API 端点
API_ENDPOINT=https://ark.cn-beijing.volces.com/api/v3/chat/completions

# 使用的模型
MODEL=ep-20240311175046-xqxz6

# 模型提供商（openai 或 aliyun）
PROVIDER=aliyun

# 解释的语言
LANGUAGE=zh

# 启用静默模式
SILENT_MODE=false

# 启用轻量 RAG（基于历史命令检索增强）
RAG_ENABLED=true

# 启用自定义 Skill（本地命令集合）
SKILLS_ENABLED=true

# 自定义 Skill 配置文件路径
SKILLS_FILE=locales/skills.json

# RAG 检索条数
RAG_TOP_K=3

# 本地检索最低命中分（低于该值才触发语义扩展）
RAG_MIN_LOCAL_HIT_SCORE=4

# 本地检索最低覆盖率（0-1，低于该值才触发语义扩展）
RAG_MIN_LOCAL_COVERAGE=0.45

# RAG 反馈权重文件路径（可选）
RAG_FEEDBACK_FILE=

# 开启模型语义扩展（本地检索未命中时触发）
RAG_SEMANTIC_EXPAND=true
```

#### 使用环境变量

```bash
# Linux/macOS
export API_KEY=your-api-key

# Windows
set API_KEY=your-api-key
```

### 运行

```bash
# 交互式模式（默认短对话）
./naturalcmd

# 启动即进入长对话模式
./naturalcmd -l

# 带参数直接生成命令
./naturalcmd "列出当前目录中的所有文件"

# 使用 p 标志指定提示
./naturalcmd -p "查找所有 .go 文件"

# 按关键词搜索历史记录
./naturalcmd -h "git push"

# 查看全部历史记录（不带值）
./naturalcmd -h

# 组合短参数示例
./naturalcmd -hs
./naturalcmd -ps "查找所有 .go 文件"
```

### 交互模式中的特殊输入

运行 `./naturalcmd` 进入交互模式后，可以使用以下特殊输入：

```bash
# 进入/退出长对话模式
-l

# 一次性执行系统命令（不经过 LLM）
!dir
!ls -la
!git status

# 进入持久命令行直连模式（所有输入当作命令执行）
!

# 在直连模式中退出回到 LLM 模式
!

# 清屏
[直接按 Enter]
```

### 标志

- `-p`: 直接指定提示
- `-h`: 按关键词搜索历史记录；当不带值时等价于查看全部历史
- `-s`: 不生成命令解释
- `-l`: 启用长对话模式；在交互过程中输入 `-l` 可进入/退出长对话模式（退出时清空该模式上下文）
- 空行行为：在主输入阶段直接按 Enter 可清屏

## 配置

该工具使用以下环境变量进行配置：

| 环境变量 | 描述 | 默认值 |
|----------|------|--------|
| `API_KEY` | 您的 API 密钥（必填） | |
| `API_ENDPOINT` | API 端点 | `https://ark.cn-beijing.volces.com/api/v3/chat/completions` |
| `MODEL` | 使用的模型 | `ep-20240311175046-xqxz6` |
| `PROVIDER` | 模型提供商（`openai` 或 `aliyun`） | `aliyun` |
| `LANGUAGE` | 解释的语言 | `zh` |
| `SILENT_MODE` | 启用静默模式 | `false` |
| `SKILLS_ENABLED` | 是否启用自定义 Skill | `true` |
| `SKILLS_FILE` | 自定义 Skill 配置文件路径 | `locales/skills.json` |
| `RAG_ENABLED` | 是否启用基于历史记录的 RAG 增强 | `true` |
| `RAG_TOP_K` | RAG 检索返回的历史条数 | `3` |
| `RAG_MIN_LOCAL_HIT_SCORE` | 本地检索最低命中分（低于该值才视为未命中） | `4` |
| `RAG_MIN_LOCAL_COVERAGE` | 本地检索最低覆盖率（0-1） | `0.45` |
| `RAG_FEEDBACK_FILE` | RAG 成功/失败反馈权重文件路径（为空则使用默认路径） | `` |
| `RAG_SEMANTIC_EXPAND` | 本地检索未命中时是否调用模型做语义扩展 | `true` |

## 工作原理

1. **用户输入**：用户输入自然语言描述的任务
2. **环境感知**：工具获取当前目录的文件和目录状态
3. **AI 处理**：将用户输入和环境信息发送给 AI 模型
4. **响应解析**：解析聊天补全响应（支持流式 SSE），实时拼接 `choices[].delta.content`（兼容 `message.content` / `output.text`），并提取最终单行命令。
5. **交互执行**：展示生成命令，用户可选择执行、继续调整（补充信息后重生成）或取消。
6. **失败闭环**：若命令执行失败，自动将错误信息、退出码和输出回灌给模型，重新生成更可靠的命令。

## 交互模式说明

NaturalCmd 提供三种交互模式：

### 1. 默认短对话模式

- **启动方式**：直接运行 `./naturalcmd`（不带 `-l` 标志）
- **特点**：每次请求独立处理，无上下文保留
- **使用场景**：单次命令生成，无需跨越多个请求的记忆

### 2. 长对话模式 (`-l`)

- **启动方式**：`./naturalcmd -l` 或在交互中输入 `-l`
- **特点**：
  - 对话上下文持续保留，直至退出
  - 后续生成的命令可以基于之前的交互历史
  - 输入 `-l` 可切换模式（再次输入 `-l` 退出，并清空本模式上下文）
- **使用场景**：需要在同一"会话"中进行多步操作，互相关联的任务

**示例**：
```
你想让我做什么？初始化 git 仓库
[生成 git init 命令]

你想让我做什么？添加所有文件
[理解上文已有 git 仓库，生成 git add . 命令]

你想让我做什么？-l
已退出长对话模式，并清空本次长对话上下文。
```

### 3. 命令行直连模式 (`!`)

分为两种用法：

#### 3a. 一次性直连执行（`!command`）

- **触发方式**：在 LLM 模式下输入 `!<命令>`
- **特点**：
  - 直接执行系统命令，跳过 LLM 处理
  - 执行完毕后回到 LLM 模式
  - 不需要生成、确认等步骤
- **使用场景**：快速执行已知命令，无需 AI 生成

**示例**：
```
你想让我做什么？!dir
正在执行命令...
Running: dir
[列出当前目录内容]

你想让我做什么？[回到 LLM 模式]
```

#### 3b. 持久直连模式（`!` 单独输入）

- **触发方式**：在任何模式下输入 `!` 单独一行
- **特点**：
  - 进入"壳直连模式"，所有非空输入都作为 Shell 命令直接执行
  - 输入 `!` 再次退出，回到原模式
  - 命令执行无需 LLM 干预，速度快
- **使用场景**：需要连续执行多个系统命令

**示例**：
```
你想让我做什么？!
已进入命令行直连模式。所有输入将直接作为 Shell 命令执行；输入 ! 可退出此模式。

命令行直连模式 > ls -la
[列出详细文件列表]

命令行直连模式 > cd src
[切换目录]

命令行直连模式 > !
已退出命令行直连模式，回到 LLM 模式。

你想让我做什么？[回到 LLM 模式]
```

### 清屏

在主输入阶段直接按 Enter（空行），可以清屏并继续等待输入。

## 终端与编码说明（Windows）

- 在 Windows 上，命令输出会做编码兼容处理，尽量避免中文乱码。
- 在支持 ANSI 控制序列的终端（如 VS Code 终端、Windows Terminal）中，清屏体验最佳。

## 智能 Skill

可以在 `locales/skills.json` 中维护常用的复杂命令流程 skill。运行时 AI 会先阅读技能目录，再根据你的提示词自主选择最相关 skill，最后综合生成命令。

示例：

```json
{
	"skills": [
		{
			"name": "project-check",
			"description": "运行测试并构建项目",
			"triggers": ["检查项目", "project check", "质量检查"],
			"commands": [
				"go test ./...",
				"go build ./..."
			],
			"continue_on_error": false,
			"priority": 10
		},
		{
			"name": "quick-clean",
			"triggers": ["快速清理"],
			"commands": [
				"go clean",
				"go mod tidy"
			],
			"continue_on_error": true
		}
	]
}
```

字段说明：

- `name`：技能名（必填）
- `description`：技能说明（可选，命中后会作为解释展示）
- `triggers`：触发词列表（可选，不填时默认使用 `name`）
- `commands`：命令集合（必填）
- `continue_on_error`：是否在某条命令失败后继续执行后续命令（默认 `false`）
- `priority`：同分命中时优先级，越大越优先（可选）

命中规则：

- 有 API Key 时：AI 会先选择 skill，再把选中 skill 的上下文和历史/RAG信息一起用于最终命令生成。
- 无 API Key 时：退化为本地触发词命中（仅执行本地 skill，无法进行模型推理）。