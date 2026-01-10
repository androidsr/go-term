package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"go-term/models"
)

// EnhancedScriptExecutor 增强的脚本执行器
type EnhancedScriptExecutor struct {
	scriptParser *ScriptParser
}

// NewEnhancedScriptExecutor 创建新的增强脚本执行器
func NewEnhancedScriptExecutor() *EnhancedScriptExecutor {
	return &EnhancedScriptExecutor{
		scriptParser: NewScriptParser(),
	}
}

// ParseCommands 解析普通命令（支持文件操作指令）
func (ese *EnhancedScriptExecutor) ParseCommands(scriptContent string) []ParsedCommand {
	rawCommands := ese.scriptParser.ParseCommands(scriptContent)
	var parsedCommands []ParsedCommand

	for _, cmd := range rawCommands {
		trimmedCmd := strings.TrimSpace(cmd)
		parsedCmd := ParsedCommand{}

		// 检查是否是文件上传命令
		if strings.HasPrefix(trimmedCmd, "$upload ") {
			parsedCmd.CommandType = "upload"
			parsedCmd.Command = strings.TrimSpace(strings.TrimPrefix(trimmedCmd, "$upload"))
		} else if strings.HasPrefix(trimmedCmd, "$download ") {
			parsedCmd.CommandType = "download"
			parsedCmd.Command = strings.TrimSpace(strings.TrimPrefix(trimmedCmd, "$download"))
		} else {
			parsedCmd.CommandType = "shell"
			parsedCmd.Command = cmd
		}

		parsedCommands = append(parsedCommands, parsedCmd)
	}

	return parsedCommands
}

// ParsedCommand 解析后的命令
type ParsedCommand struct {
	Command     string // 命令内容
	CommandType string // 命令类型: shell, upload, download
}

// ExecuteScriptMode 脚本模式执行 - 将整个脚本内容作为一个整体执行
func (ese *EnhancedScriptExecutor) ExecuteScriptMode(
	scriptContent string,
	executor CommandExecutor,
	serverID string,
) ([]models.CommandOutput, error) {
	now := time.Now().Format("2006-01-02 15:04:05")

	// 在脚本模式中，需要预处理文件操作命令
	processedScript, fileOperations := ese.preprocessScriptForFileOperations(scriptContent)

	// 如果有文件操作命令，使用命令模式执行所有命令（保持原始顺序）
	if len(fileOperations) > 0 {
		// 使用命令模式执行，这样可以按原始顺序执行所有命令
		return ese.ExecuteCommandMode(fileOperations, executor, serverID)
	}

	// 没有文件操作时，正常执行脚本
	var commandOutputs []models.CommandOutput
	cmdOutput := models.CommandOutput{
		Command:   "[完整脚本执行]",
		Status:    "running",
		StartTime: now,
	}

	// 执行处理后的脚本内容（使用直接执行，不通过终端会话）
	output, err := executor.ExecCommandDirect(serverID, processedScript)
	cmdOutput.EndTime = time.Now().Format("2006-01-02 15:04:05")
	cmdOutput.Output = output

	if err != nil {
		cmdOutput.Status = "failed"
		// 清理错误信息，避免重复包装
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "执行命令失败:") {
			parts := strings.SplitN(errorMsg, ":", 3)
			if len(parts) >= 3 {
				errorMsg = strings.TrimSpace(parts[2])
			}
		}

		// 尝试从错误信息中提取行号
		lineInfo := ese.extractLineInfoFromError(errorMsg, scriptContent)
		if lineInfo != "" {
			cmdOutput.Error = fmt.Sprintf("脚本执行失败 (第%s行): %s", lineInfo, errorMsg)
		} else {
			cmdOutput.Error = fmt.Sprintf("脚本执行失败: %s", errorMsg)
		}

		// 确保输出字段包含错误信息，这样前端能显示
		if output == "" {
			cmdOutput.Output = cmdOutput.Error
		} else {
			// 即使有输出，也要确保错误信息被包含
			cmdOutput.Output = fmt.Sprintf("%s\n错误信息: %s", output, cmdOutput.Error)
		}
	} else {
		cmdOutput.Status = "success"
		// 确保即使成功也有输出内容显示
		if output == "" {
			cmdOutput.Output = "脚本执行完成，无输出内容"
		}
	}

	commandOutputs = append(commandOutputs, cmdOutput)
	return commandOutputs, nil
}

// extractLineInfoFromError 从错误信息中提取行号信息
func (ese *EnhancedScriptExecutor) extractLineInfoFromError(errorMsg, scriptContent string) string {
	// 常见的错误行号模式匹配
	patterns := []string{
		`line (\d+)`,
		`行 (\d+)`,
		`at line (\d+)`,
		`第(\d+)行`,
		`syntax error at line (\d+)`,
		`error on line (\d+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(errorMsg)
		if len(matches) >= 2 {
			return matches[1]
		}
	}

	// 如果错误信息中没有明确的行号，尝试通过上下文推断
	lines := strings.Split(scriptContent, "\n")
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "#") {
			// 提取命令的第一个词
			fields := strings.Fields(trimmedLine)
			if len(fields) > 0 {
				firstWord := fields[0]
				// 检查错误信息中是否包含该命令关键字
				if strings.Contains(errorMsg, firstWord) {
					return fmt.Sprintf("%d", i+1)
				}
			}
		}
	}

	return ""
}

// preprocessScriptForFileOperations 预处理脚本，提取文件操作命令
func (ese *EnhancedScriptExecutor) preprocessScriptForFileOperations(scriptContent string) (string, []ParsedCommand) {
	// 解析所有命令
	commands := ese.scriptParser.ParseCommands(scriptContent)
	var fileOperations []ParsedCommand
	var hasFileOperations bool

	// 分类命令并按原始顺序创建混合命令列表
	var mixedCommands []ParsedCommand

	for _, cmd := range commands {
		trimmedCmd := strings.TrimSpace(cmd)
		parsedCmd := ParsedCommand{}

		if strings.HasPrefix(trimmedCmd, "$upload ") {
			parsedCmd.CommandType = "upload"
			parsedCmd.Command = strings.TrimSpace(strings.TrimPrefix(trimmedCmd, "$upload"))
			fileOperations = append(fileOperations, parsedCmd)
			mixedCommands = append(mixedCommands, parsedCmd)
			hasFileOperations = true
		} else if strings.HasPrefix(trimmedCmd, "$download ") {
			parsedCmd.CommandType = "download"
			parsedCmd.Command = strings.TrimSpace(strings.TrimPrefix(trimmedCmd, "$download"))
			fileOperations = append(fileOperations, parsedCmd)
			mixedCommands = append(mixedCommands, parsedCmd)
			hasFileOperations = true
		} else {
			// 普通shell命令
			parsedCmd.CommandType = "shell"
			parsedCmd.Command = cmd
			mixedCommands = append(mixedCommands, parsedCmd)
		}
	}

	// 如果没有文件操作，返回原脚本和空列表
	if !hasFileOperations {
		return scriptContent, []ParsedCommand{}
	}

	// 构建不包含文件操作的脚本内容用于shell执行
	var shellCommands []string
	for _, cmd := range mixedCommands {
		if cmd.CommandType == "shell" {
			shellCommands = append(shellCommands, cmd.Command)
		}
	}

	var shellScript string
	if len(shellCommands) > 0 {
		shellScript = strings.Join(shellCommands, "\n")
	}

	return shellScript, mixedCommands
}

// handleUploadCommand 处理文件上传命令
func (ese *EnhancedScriptExecutor) handleUploadCommand(executor CommandExecutor, serverID, command string) (string, error) {
	// 解析命令参数: 本地文件路径 远程保存目录
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return "", fmt.Errorf("上传命令格式错误: $upload 本地文件路径 远程保存目录")
	}

	localPath := parts[0]
	remoteDir := parts[1]

	// 构造远程文件路径
	// 从本地文件路径中提取文件名
	localFileName := localPath
	if idx := strings.LastIndex(localPath, "/"); idx != -1 {
		localFileName = localPath[idx+1:]
	} else if idx := strings.LastIndex(localPath, "\\"); idx != -1 {
		localFileName = localPath[idx+1:]
	}

	remotePath := remoteDir
	if !strings.HasSuffix(remoteDir, "/") {
		remotePath += "/"
	}
	remotePath += localFileName

	// 确保SFTP客户端已创建
	err := executor.EnsureSFTPClient(serverID)
	if err != nil {
		return "", fmt.Errorf("创建SFTP客户端失败: %v", err)
	}

	// 执行上传操作
	_, err = executor.ExecUploadFile(serverID, localPath, remotePath)
	if err != nil {
		return "", fmt.Errorf("文件上传失败: %v", err)
	}

	return fmt.Sprintf("文件上传成功: %s -> %s", localPath, remotePath), nil
}

// handleDownloadCommand 处理文件下载命令
func (ese *EnhancedScriptExecutor) handleDownloadCommand(executor CommandExecutor, serverID, command string) (string, error) {
	// 解析命令参数: 远程文件路径 本地保存路径
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return "", fmt.Errorf("下载命令格式错误: $download 远程文件路径 本地保存路径")
	}

	remotePath := parts[0]
	localPath := parts[1]

	// 确保SFTP客户端已创建
	err := executor.EnsureSFTPClient(serverID)
	if err != nil {
		return "", fmt.Errorf("创建SFTP客户端失败: %v", err)
	}

	// 执行下载操作
	_, err = executor.ExecDownloadFile(serverID, remotePath, localPath)
	if err != nil {
		return "", fmt.Errorf("文件下载失败: %v", err)
	}

	return fmt.Sprintf("文件下载成功: %s -> %s", remotePath, localPath), nil
}

// ExecuteCommandMode 命令模式执行 - 逐条执行每个命令
func (ese *EnhancedScriptExecutor) ExecuteCommandMode(
	commands []ParsedCommand,
	executor CommandExecutor,
	serverID string,
) ([]models.CommandOutput, error) {
	var commandOutputs []models.CommandOutput
	now := time.Now().Format("2006-01-02 15:04:05")

	// 将命令分为两类：文件操作命令和shell命令
	var fileOps []ParsedCommand
	var shellCommands []string

	for _, parsedCmd := range commands {
		if parsedCmd.CommandType == "upload" || parsedCmd.CommandType == "download" {
			fileOps = append(fileOps, parsedCmd)
		} else if parsedCmd.CommandType == "shell" {
			shellCommands = append(shellCommands, parsedCmd.Command)
		}
	}

	// 先执行文件操作命令
	for _, parsedCmd := range fileOps {
		cmdOutput := models.CommandOutput{
			Command:   parsedCmd.Command,
			Status:    "running",
			StartTime: now,
		}

		var err error
		var output string

		if parsedCmd.CommandType == "upload" {
			output, err = ese.handleUploadCommand(executor, serverID, parsedCmd.Command)
		} else if parsedCmd.CommandType == "download" {
			output, err = ese.handleDownloadCommand(executor, serverID, parsedCmd.Command)
		}

		cmdOutput.EndTime = time.Now().Format("2006-01-02 15:04:05")
		cmdOutput.Output = output

		if err != nil {
			cmdOutput.Status = "failed"
			errorMsg := err.Error()
			if strings.Contains(errorMsg, "执行命令失败:") {
				parts := strings.SplitN(errorMsg, ":", 3)
				if len(parts) >= 3 {
					errorMsg = strings.TrimSpace(parts[2])
				}
			}
			if parsedCmd.CommandType == "upload" {
				cmdOutput.Error = fmt.Sprintf("文件上传失败: %s", errorMsg)
			} else {
				cmdOutput.Error = fmt.Sprintf("文件下载失败: %s", errorMsg)
			}
			if output == "" {
				cmdOutput.Output = cmdOutput.Error
			}
			commandOutputs = append(commandOutputs, cmdOutput)
			return commandOutputs, fmt.Errorf("文件操作失败")
		} else {
			cmdOutput.Status = "success"
			commandOutputs = append(commandOutputs, cmdOutput)
		}
	}

	// 然后在一个共享的session中执行所有shell命令
	if len(shellCommands) > 0 {
		outputs, err := executor.ExecCommandsInSharedSession(serverID, shellCommands)
		if err != nil {
			// 失败时，为所有shell命令添加失败记录
			for i, cmd := range shellCommands {
				cmdOutput := models.CommandOutput{
					Command:   cmd,
					Status:    "failed",
					StartTime: now,
					EndTime:   time.Now().Format("2006-01-02 15:04:05"),
				}
				cmdOutput.Error = err.Error()
				if i < len(outputs) {
					cmdOutput.Output = outputs[i]
				} else {
					cmdOutput.Output = cmdOutput.Error
				}
				commandOutputs = append(commandOutputs, cmdOutput)
			}
			return commandOutputs, err
		}

		// 成功时，为每个shell命令添加成功记录
		// outputs 数组已经包含了每个命令的输出
		for i, cmd := range shellCommands {
			cmdOutput := models.CommandOutput{
				Command:   cmd,
				Status:    "success",
				StartTime: now,
				EndTime:   time.Now().Format("2006-01-02 15:04:05"),
			}
			if i < len(outputs) {
				cmdOutput.Output = outputs[i]
			} else {
				cmdOutput.Output = "命令执行完成，无输出"
			}
			commandOutputs = append(commandOutputs, cmdOutput)
		}
	}

	return commandOutputs, nil
}

// ExecuteCommands 执行命令列表（保持向后兼容，使用命令模式）
func (ese *EnhancedScriptExecutor) ExecuteCommands(
	commands []ParsedCommand,
	executor CommandExecutor,
	serverID string,
) ([]models.CommandOutput, error) {
	return ese.ExecuteCommandMode(commands, executor, serverID)
}

// CommandExecutor 命令执行接口
type CommandExecutor interface {
	ExecCommand(serverID, command string) (string, error)
	ExecUploadFile(serverID, localPath, remotePath string) (string, error)
	ExecDownloadFile(serverID, remotePath, localPath string) (string, error)
	EnsureSFTPClient(serverID string) error // 确保SFTP客户端已创建
	ExecCommandDirect(serverID, command string) (string, error) // 直接执行命令（不通过终端会话）
	ExecCommandsInSharedSession(serverID string, commands []string) ([]string, error) // 在同一个session中执行多个命令
}
