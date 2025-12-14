package services

import (
	"bufio"
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
