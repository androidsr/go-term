package services

import (
	"bufio"
	"fmt"
	"strings"
)

// ScriptParser 脚本解析器
type ScriptParser struct{}

// NewScriptParser 创建脚本解析器
func NewScriptParser() *ScriptParser {
	return &ScriptParser{}
}

// ParseCommands 解析脚本内容，提取有效命令
func (sp *ScriptParser) ParseCommands(scriptContent string) []string {
	if scriptContent == "" {
		return []string{}
	}

	var commands []string
	scanner := bufio.NewScanner(strings.NewReader(scriptContent))

	var currentCommand strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行
		if line == "" {
			continue
		}

		// 跳过注释行 (以 # 或 // 开头)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// 检查是否是多行命令的延续 (行末有 \)
		if strings.HasSuffix(line, "\\") {
			// 移除末尾的 \ 并添加到当前命令
			currentCommand.WriteString(strings.TrimSuffix(line, "\\") + " ")
		} else {
			// 添加当前行到命令
			currentCommand.WriteString(line)
			command := strings.TrimSpace(currentCommand.String())

			if command != "" {
				commands = append(commands, command)
				currentCommand.Reset()
			}
		}
	}

	// 检查是否还有未完成的命令
	if currentCommand.Len() > 0 {
		command := strings.TrimSpace(currentCommand.String())
		if command != "" {
			commands = append(commands, command)
		}
	}

	return commands
}

// BuildCombinedScript 将多个命令组合为在同一会话中执行的脚本
func (sp *ScriptParser) BuildCombinedScript(commands []ParsedCommand) string {
	if len(commands) == 0 {
		return ""
	}

	// 构建一个完整的shell脚本，确保所有命令在同一会话中执行
	var script strings.Builder
	script.WriteString("#!/bin/bash\n")
	// 不使用 set -e，这样即使某个命令失败，后续命令也会继续执行，便于我们记录所有结果
	// script.WriteString("set -e  # 遇到错误时退出\n")
	script.WriteString("\n")

	for i, command := range commands {
		// 为每个命令添加标识，便于输出解析
		script.WriteString(fmt.Sprintf("echo \"[COMMAND %d] %s\"\n", i+1, command.Command))
		script.WriteString(command.Command)
		script.WriteString("\n")
		script.WriteString("EXIT_CODE=$?\n")
		script.WriteString("echo \"[COMMAND_EXIT_CODE:$EXIT_CODE]\"\n") // 记录退出码

		// 如果命令没有设置继续执行标记，则在失败时退出脚本
		if !command.ContinueOnError {
			script.WriteString("if [ $EXIT_CODE -ne 0 ]; then\n")
			script.WriteString("  echo \"[COMMAND_STOPPED_DUE_TO_FAILURE]\"\n")
			script.WriteString("  exit $EXIT_CODE\n")
			script.WriteString("fi\n")
		}

		script.WriteString("echo \"[COMMAND_SEPARATOR]\"\n") // 命令分隔符
		script.WriteString("\n")
	}

	return script.String()
}

// IsValidCommand 检查是否是有效命令（不是注释或空行）
func (sp *ScriptParser) IsValidCommand(line string) bool {
	trimmed := strings.TrimSpace(line)

	// 空行
	if trimmed == "" {
		return false
	}

	// 注释行
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
		return false
	}

	return true
}
