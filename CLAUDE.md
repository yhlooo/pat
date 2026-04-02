# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目简介

PAT (Polymarket Automated Trading System) 是一个基于 Go 语言构建的 Polymarket 自动交易系统。

## 技术栈

- 使用 go 编写
- 主要第三方库：
  - AI Agent 框架： `github.com/firebase/genkit/go`
  - Agent 和 TUI 使用 ACP 协议交互： `github.com/coder/acp-go-sdk`
  - 命令行入口： `github.com/spf13/cobra`
  - TUI 框架： `github.com/charmbracelet/bubbletea`
  - 日志： `github.com/go-logr/logr`
  - 单元测试断言： `github.com/stretchr/testify`
  - 国际化： `github.com/nicksnyder/go-i18n/v2/i18n`

## 项目结构

```
.
├── cmd
│   └── pat  # 程序入口， main 包 
└── pkg  # 代码
  ├── commands    # 命令行入口实现
  ├── configs     # 配置系统实现
  ├── i8n         # 国际化
  ├── polymarket  # Polymarket 客户端等实现
  ├── trading     # 监听市场、交易、策略实现
  ├── ui          # 用户交互界面实现
  # ... 其它包
```

## 提案规范

- 所有 OpenSpec artifacts 都必须使用中文编写

## 代码质量检查

编辑代码后，必须按以下顺序执行检查：

```bash
# 1. 代码格式化（必须在编辑后立即执行）
go fmt ./...
# 2. 静态分析检查语法问题（使用 go vet 而非 go build）
go vet ./...
# 3. 运行单元测试确认功能正常
go test ./...
```

**说明**：
- `go fmt ./...` - 自动格式化所有 Go 代码，确保代码风格一致
- `go vet ./...` - 检查代码中的常见错误，而不是使用 `go build`
- `go test ./...` - 运行所有单元测试，确保修改没有破坏现有功能
- **禁止** 使用 `go build` ，构建没有必要，而且产生预期之外的产物，且 vet 能发现 build 无法检测的问题

## 国际化

原则上对用户展示的文本（不含日志）都需要支持中文和英文两种语言。这些需要支持国际化的文本需要以 `i18n.Message` 结构体的形式定义在同一个包中的 `i18n.go` 文件中，比如 CLI 命令相关的描述文本定义在 `pkg/commands/i18n.go` 中。

翻译文件在 `pkg/i18n/active.en.yaml` 和 `pkg/i18n/active.zh.yaml` 中，但是这两个文件 **不能直接修改** ，需要通过 Skill `i18n-translate` 从代码中提取 `i18n.Message` 生成。

## 外部参考资料

- PolyMarket: https://docs.polymarket.com/llms.txt （建议通过 curl 访问）
