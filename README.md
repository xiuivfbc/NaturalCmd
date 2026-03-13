# NaturalCmd

一个用 Go 实现的 Cmd，将自然语言转换为 Cmd 命令的命令行工具。

## 功能

- 将自然语言转换为 Cmd 命令
- 自动执行生成的命令
- 为生成的命令提供解释
- 跨平台支持
- 流式响应，提供更好的用户体验
- 环境感知能力，能够获取当前目录文件状态
- 智能错误处理，执行失败时会自动重新生成脚本
- 支持多语言输出（中文/英文）
- 支持不同的 AI 模型提供商（OpenAI、阿里云）

## 安装

### 前提条件

- Go 1.20 或更高版本
- AI 模型 API 密钥（OpenAI 或阿里云）

### 构建

```bash
go build -o ai ./cmd/ai
```

## 使用

### 设置 API 密钥h

#### 使用 .env 文件（推荐）

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
# 带参数运行
./ai "列出当前目录中的所有文件"

# 使用 prompt 标志
./ai --prompt "查找所有 .go 文件"

# 交互式模式（不带参数）
./ai
```

### 标志

- `-prompt`, `-p`: 直接指定提示
- `-silent`, `-s`: 跳过打印命令解释

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

## 工作原理

1. **用户输入**：用户输入自然语言描述的任务
2. **环境感知**：工具获取当前目录的文件和目录状态
3. **AI 处理**：将用户输入和环境信息发送给 AI 模型
4. **响应解析**：解析 AI 模型返回的 JSON 响应，包含以下字段：
   - `object`：目标对象（`system` 或 `user`）
   - `action`：操作类型（`execute`、`request_info`、`display`）
   - `content`：命令内容或显示信息
   - `info_needed`：请求的信息类型（当 action 为 `request_info` 时）
5. **执行操作**：
   - 如果 `object` 为 `system`，自动执行命令并将结果返回给 AI
   - 如果 `object` 为 `user`，让用户选择是否执行命令
   - 如果 `action` 为 `request_info`，获取请求的信息并返回给 AI
   - 如果 `action` 为 `display`，直接显示信息给用户或返回给 AI
6. **循环处理**：如果执行失败或需要更多信息，自动重新生成脚本

## 示例

### 使用阿里云（默认）

```bash
$ ./ai "列出当前目录中的所有文件"
正在生成脚本...

生成的脚本：ls -la

正在生成解释...

解释：
- 列出当前目录中的所有文件和目录
- 包括隐藏文件（以 . 开头的文件）
- 显示详细信息，包括权限、所有者、大小和修改时间

? 你想要执行这个脚本吗？(要得/不得行) 要得

正在执行命令...
Running: ls -la
total 32
drwxr-xr-x  5 user  group  160 Mar 11 12:00 .
drwxr-xr-x  3 user  group   96 Mar 11 11:50 ..
-rwxr-xr-x  1 user  group 8944 Mar 11 12:00 ai
-rw-r--r--  1 user  group  116 Mar 11 11:50 go.mod
drwxr-xr-x  3 user  group   96 Mar 11 11:50 internal
```

### 使用 OpenAI

```bash
$ export API_KEY=your-openai-api-key
$ export API_ENDPOINT=https://api.openai.com/v1/chat/completions
$ export MODEL=gpt-4o-mini
$ export PROVIDER=openai
$ ./ai "列出当前目录中的所有文件"
正在生成脚本...

生成的脚本：ls -la

正在生成解释...

解释：
- 列出当前目录中的所有文件和目录
- 包括隐藏文件（以 . 开头的文件）
- 显示详细信息，包括权限、所有者、大小和修改时间

? 你想要执行这个脚本吗？(要得/不得行) 要得

正在执行命令...
Running: ls -la
total 32
drwxr-xr-x  5 user  group  160 Mar 11 12:00 .
drwxr-xr-x  3 user  group   96 Mar 11 11:50 ..
-rwxr-xr-x  1 user  group 8944 Mar 11 12:00 ai
-rw-r--r--  1 user  group  116 Mar 11 11:50 go.mod
drwxr-xr-x  3 user  group   96 Mar 11 11:50 internal
```

## 常见问题

### 命令执行失败怎么办？

如果命令执行失败，工具会自动将错误信息添加到提示中，让 AI 重新生成脚本。

### 如何切换语言？

可以通过设置 `LANGUAGE` 环境变量来切换语言，支持 `zh`（中文）和 `en`（英文）。

### 如何使用其他 AI 模型？

可以通过设置 `MODEL` 环境变量来使用其他 AI 模型，同时需要设置相应的 `API_ENDPOINT` 和 `PROVIDER`。

## 贡献

欢迎提交问题和拉取请求！