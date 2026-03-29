---
paths:
  - "pkg/commands/*.go"
---

# CLI 子命令规范

## 子命令与文件结构映射

- 根命令定义在 `pkg/commands/root.go` 中
- 每个一级子命令（比如 `pat version ...` ）及其子命令应至少在 `pkg/commands/` 目录下有一个文件，以一级子命令名命名

## 子命令定义

每个子命令定义都类似以下内容（可以参考 `pkg/commands/version.go` ）：

- 以子命令命名的 `XxxOptions` 结构体，定义子命令绑定到命令行 flag 的选项。需要有一个 `AddPFlags(fs *pflag.FlagSet)` 成员方法，用于把结构体字段绑定到命令行 flag
- 方法 `NewXxxOptions() XxxOptions` 用于创建带默认值的 `XxxOptions`
- 以子命令命名的 `newXxxCommand() *cobra.Command` 用于创建子命令的 `*cobra.Command` 对象， `*cobra.Command` 对象应包含以下字段：
  - `Use` 子命令用法示例，子命令名和位置参数占位符
  - `Short` 子命令的简短描述
  - `Args` 位置参数数量定义
  - `RunE` （可选）执行该命令的逻辑
