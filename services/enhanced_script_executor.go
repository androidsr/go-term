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
		// 如果前面的命令失败且没有设置继续执行，则跳过后续命令的执行和显示
		if shouldStop {
			cmdOutput := models.CommandOutput{
				Command:   parsedCmd.Command,
				Status:    "skipped",
				StartTime: now,
				EndTime:   now,
				Error:     "前面的命令执行失败，已跳过此命令",
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
