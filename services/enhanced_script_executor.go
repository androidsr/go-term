package services

import (
	"fmt"
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

// ParseCommandsWithSpecialHandling 解析命令并处理特殊功能
func (ese *EnhancedScriptExecutor) ParseCommandsWithSpecialHandling(scriptContent string) []ParsedCommand {
	rawCommands := ese.scriptParser.ParseCommands(scriptContent)
	var parsedCommands []ParsedCommand

	for _, cmd := range rawCommands {
		parsedCmd := ParsedCommand{
			Command:         cmd,
			ContinueOnError: strings.HasSuffix(strings.TrimSpace(cmd), "$ne"),
		}

		// 如果是特殊命令，去除标记后缀
		if parsedCmd.ContinueOnError {
			// 去除 $ne 后缀
			parsedCmd.Command = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(cmd), "$ne"))
		}

		// 检查是否是文件上传命令
		if strings.HasPrefix(parsedCmd.Command, "$upload ") {
			parsedCmd.CommandType = "upload"
			parsedCmd.Command = strings.TrimSpace(strings.TrimPrefix(parsedCmd.Command, "$upload"))
		} else if strings.HasPrefix(parsedCmd.Command, "$download ") {
			parsedCmd.CommandType = "download"
			parsedCmd.Command = strings.TrimSpace(strings.TrimPrefix(parsedCmd.Command, "$download"))
		} else {
			parsedCmd.CommandType = "shell"
		}

		parsedCommands = append(parsedCommands, parsedCmd)
	}

	return parsedCommands
}

// ParsedCommand 解析后的命令
type ParsedCommand struct {
	Command         string // 命令内容
	CommandType     string // 命令类型: shell, upload, download
	ContinueOnError bool   // 是否在错误时继续执行
}

// ExecuteCommands 执行命令列表
func (ese *EnhancedScriptExecutor) ExecuteCommands(
	commands []ParsedCommand,
	executor CommandExecutor,
	serverID string,
) ([]models.CommandOutput, error) {
	var commandOutputs []models.CommandOutput
	now := time.Now().Format("2006-01-02 15:04:05")

	shouldStop := false

	for _, parsedCmd := range commands {
		// 如果前面的命令失败且没有设置继续执行，则停止执行后续命令
		if shouldStop {
			// 即使停止执行，也要添加一个跳过的记录
			cmdOutput := models.CommandOutput{
				Command:   parsedCmd.Command,
				Status:    "skipped",
				StartTime: now,
				EndTime:   time.Now().Format("2006-01-02 15:04:05"),
			}
			commandOutputs = append(commandOutputs, cmdOutput)
			continue
		}

		cmdOutput := models.CommandOutput{
			Command:   parsedCmd.Command,
			Status:    "running",
			StartTime: now,
		}

		var err error
		var output string

		switch parsedCmd.CommandType {
		case "upload":
			output, err = ese.handleUploadCommand(executor, serverID, parsedCmd.Command)
		case "download":
			output, err = ese.handleDownloadCommand(executor, serverID, parsedCmd.Command)
		default:
			// 执行普通shell命令
			output, err = executor.ExecCommand(serverID, parsedCmd.Command)
		}

		cmdOutput.EndTime = time.Now().Format("2006-01-02 15:04:05")
		cmdOutput.Output = output

		if err != nil {
			cmdOutput.Status = "failed"
			cmdOutput.Error = err.Error()

			// 保存详细的错误信息
			if output != "" {
				cmdOutput.Error = fmt.Sprintf("%s\n详细输出:\n%s", err.Error(), output)
			}

			// 如果命令没有设置继续执行标记，则停止后续命令执行
			if !parsedCmd.ContinueOnError {
				shouldStop = true
			}
		} else {
			cmdOutput.Status = "success"
		}

		commandOutputs = append(commandOutputs, cmdOutput)
	}

	return commandOutputs, nil
}

// ExecuteCommandsWithSameSession 在同一个会话中执行命令列表
// 这个方法确保所有shell命令在同一个SSH会话中执行，以保持上下文连贯性
func (ese *EnhancedScriptExecutor) ExecuteCommandsWithSameSession(
	commands []ParsedCommand,
	executor CommandExecutor,
	serverID string,
) ([]models.CommandOutput, error) {
	var commandOutputs []models.CommandOutput
	now := time.Now().Format("2006-01-02 15:04:05")

	shouldStop := false

	// 收集所有shell命令并构建组合脚本
	var shellCommands []ParsedCommand
	for _, cmd := range commands {
		if cmd.CommandType == "shell" {
			shellCommands = append(shellCommands, cmd)
		}
	}

	// 如果有shell命令，在同一个会话中执行它们
	var shellResults []models.CommandOutput
	if len(shellCommands) > 0 {
		// 构建组合脚本
		combinedScript := ese.scriptParser.BuildCombinedScript(shellCommands)

		// 执行组合脚本
		output, err := executor.ExecCommand(serverID, combinedScript)

		// 解析组合脚本的输出
		shellResults = parseCombinedScriptOutput(output, err, shellCommands)
	}

	// 处理所有命令，包括shell命令的结果和特殊命令
	shellIndex := 0
	for _, parsedCmd := range commands {
		// 如果前面的命令失败且没有设置继续执行，则停止执行后续命令
		if shouldStop {
			// 即使停止执行，也要添加一个跳过的记录
			cmdOutput := models.CommandOutput{
				Command:   parsedCmd.Command,
				Status:    "skipped",
				StartTime: now,
				EndTime:   time.Now().Format("2006-01-02 15:04:05"),
			}
			commandOutputs = append(commandOutputs, cmdOutput)
			continue
		}

		cmdOutput := models.CommandOutput{
			Command:   parsedCmd.Command,
			Status:    "running",
			StartTime: now,
		}

		var err error
		var output string

		switch parsedCmd.CommandType {
		case "upload":
			output, err = ese.handleUploadCommand(executor, serverID, parsedCmd.Command)
			cmdOutput.EndTime = time.Now().Format("2006-01-02 15:04:05")
			cmdOutput.Output = output

			if err != nil {
				cmdOutput.Status = "failed"
				cmdOutput.Error = err.Error()
				if output != "" {
					cmdOutput.Error = fmt.Sprintf("%s\n详细输出:\n%s", err.Error(), output)
				}
				if !parsedCmd.ContinueOnError {
					shouldStop = true
				}
			} else {
				cmdOutput.Status = "success"
			}
		case "download":
			output, err = ese.handleDownloadCommand(executor, serverID, parsedCmd.Command)
			cmdOutput.EndTime = time.Now().Format("2006-01-02 15:04:05")
			cmdOutput.Output = output

			if err != nil {
				cmdOutput.Status = "failed"
				cmdOutput.Error = err.Error()
				if output != "" {
					cmdOutput.Error = fmt.Sprintf("%s\n详细输出:\n%s", err.Error(), output)
				}
				if !parsedCmd.ContinueOnError {
					shouldStop = true
				}
			} else {
				cmdOutput.Status = "success"
			}
		default:
			// 使用在同一个会话中执行的结果
			if shellIndex < len(shellResults) {
				cmdOutput = shellResults[shellIndex]
				// 检查是否应该停止后续命令的执行
				if cmdOutput.Status == "failed" && !parsedCmd.ContinueOnError {
					shouldStop = true
				}
				shellIndex++
			} else {
				// fallback到单独执行命令
				output, err = executor.ExecCommand(serverID, parsedCmd.Command)
				cmdOutput.EndTime = time.Now().Format("2006-01-02 15:04:05")
				cmdOutput.Output = output

				if err != nil {
					cmdOutput.Status = "failed"
					cmdOutput.Error = err.Error()
					if output != "" {
						cmdOutput.Error = fmt.Sprintf("%s\n详细输出:\n%s", err.Error(), output)
					}
					if !parsedCmd.ContinueOnError {
						shouldStop = true
					}
				} else {
					cmdOutput.Status = "success"
				}
			}
		}

		commandOutputs = append(commandOutputs, cmdOutput)
	}

	return commandOutputs, nil
}

// parseCombinedScriptOutput 解析组合脚本的输出
func parseCombinedScriptOutput(output string, err error, commands []ParsedCommand) []models.CommandOutput {
	var results []models.CommandOutput
	now := time.Now().Format("2006-01-02 15:04:05")
	endTime := time.Now().Format("2006-01-02 15:04:05")

	if err != nil {
		// 检查是否是因为命令失败而停止执行
		if strings.Contains(output, "[COMMAND_STOPPED_DUE_TO_FAILURE]") {
			// 解析输出以确定哪个命令失败
			lines := strings.Split(output, "\n")
			failedCommandIndex := -1

			for i, line := range lines {
				if strings.Contains(line, "[COMMAND_EXIT_CODE:") && !strings.Contains(line, "[COMMAND_EXIT_CODE:0]") {
					// 找到非零退出码
					failedCommandIndex = i
					break
				}
			}

			// 为所有命令创建结果记录
			for i, cmd := range commands {
				result := models.CommandOutput{
					Command:   cmd.Command,
					StartTime: now,
					EndTime:   endTime,
				}

				if i < failedCommandIndex {
					// 失败命令之前的命令应该是成功的
					result.Status = "success"
					result.Output = ""
				} else if i == failedCommandIndex {
					// 失败的命令
					result.Status = "failed"
					result.Error = fmt.Errorf("命令执行失败").Error()
					result.Output = output
				} else {
					// 失败命令之后的命令应该是被跳过的
					result.Status = "skipped"
				}

				results = append(results, result)
			}
			return results
		} else {
			// 其他错误情况
			for _, cmd := range commands {
				result := models.CommandOutput{
					Command:   cmd.Command,
					Status:    "failed",
					StartTime: now,
					EndTime:   endTime,
					Output:    output,
					Error:     err.Error(),
				}
				if output != "" {
					result.Error = fmt.Sprintf("%s\n详细输出:\n%s", err.Error(), output)
				}
				results = append(results, result)
			}
			return results
		}
	}

	if output == "" {
		// 如果没有输出，为所有命令创建成功记录（假设成功）
		for _, cmd := range commands {
			result := models.CommandOutput{
				Command:   cmd.Command,
				Status:    "success",
				StartTime: now,
				EndTime:   endTime,
				Output:    "",
			}
			results = append(results, result)
		}
		return results
	}

	// 解析组合脚本的输出
	lines := strings.Split(output, "\n")

	currentResult := models.CommandOutput{}
	var outputLines []string
	commandIndex := 0
	stoppedDueToFailure := false

	for _, line := range lines {
		if strings.Contains(line, "[COMMAND_STOPPED_DUE_TO_FAILURE]") {
			stoppedDueToFailure = true
			// 保存当前命令结果
			if currentResult.Command != "" {
				currentResult.Output = strings.Join(outputLines, "\n")
				// 如果没有设置状态，默认为成功
				if currentResult.Status == "" {
					currentResult.Status = "success"
				}
				results = append(results, currentResult)
				commandIndex++
			}
			break
		} else if strings.HasPrefix(line, "[COMMAND ") {
			// 如果已经有当前结果，保存它
			if currentResult.Command != "" {
				currentResult.Output = strings.Join(outputLines, "\n")
				// 如果没有设置状态，默认为成功
				if currentResult.Status == "" {
					currentResult.Status = "success"
				}
				results = append(results, currentResult)

				// 准备下一个结果
				commandIndex++
				currentResult = models.CommandOutput{}
				outputLines = []string{}
			}

			// 提取命令信息
			endIndex := strings.Index(line, "]")
			if endIndex > 0 && commandIndex < len(commands) {
				currentResult.Command = commands[commandIndex].Command
				currentResult.StartTime = now
				currentResult.EndTime = endTime
			}
		} else if strings.HasPrefix(line, "[COMMAND_EXIT_CODE:") {
			// 提取退出码
			codeStr := strings.TrimPrefix(line, "[COMMAND_EXIT_CODE:")
			codeStr = strings.TrimSuffix(codeStr, "]")
			if codeStr == "0" {
				currentResult.Status = "success"
			} else {
				currentResult.Status = "failed"
				currentResult.Error = fmt.Errorf("命令执行失败，退出码: %s", codeStr).Error()
			}
		} else if line == "[COMMAND_SEPARATOR]" {
			// 命令分隔符，不做处理
		} else {
			// 输出内容
			if currentResult.Command != "" {
				outputLines = append(outputLines, line)
			}
		}
	}

	// 保存最后一个命令的结果
	if currentResult.Command != "" && !stoppedDueToFailure {
		currentResult.Output = strings.Join(outputLines, "\n")
		// 如果没有设置状态，默认为成功
		if currentResult.Status == "" {
			currentResult.Status = "success"
		}
		results = append(results, currentResult)
		commandIndex++
	}

	// 如果脚本因失败而停止，后续命令标记为跳过
	if stoppedDueToFailure {
		for commandIndex < len(commands) {
			result := models.CommandOutput{
				Command:   commands[commandIndex].Command,
				Status:    "skipped",
				StartTime: now,
				EndTime:   endTime,
				Output:    "",
			}
			results = append(results, result)
			commandIndex++
		}
	} else {
		// 如果还有剩余的命令没有结果，为它们创建默认的成功记录
		for commandIndex < len(commands) {
			result := models.CommandOutput{
				Command:   commands[commandIndex].Command,
				Status:    "success",
				StartTime: now,
				EndTime:   endTime,
				Output:    "",
			}
			results = append(results, result)
			commandIndex++
		}
	}

	return results
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
	result, err := executor.ExecUploadFile(serverID, localPath, remotePath)
	if err != nil {
		return "", fmt.Errorf("文件上传失败: %v", err)
	}

	return result, nil
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
	result, err := executor.ExecDownloadFile(serverID, remotePath, localPath)
	if err != nil {
		return "", fmt.Errorf("文件下载失败: %v", err)
	}

	return result, nil
}

// CommandExecutor 命令执行接口
type CommandExecutor interface {
	ExecCommand(serverID, command string) (string, error)
	ExecUploadFile(serverID, localPath, remotePath string) (string, error)
	ExecDownloadFile(serverID, remotePath, localPath string) (string, error)
	EnsureSFTPClient(serverID string) error // 确保SFTP客户端已创建
}
