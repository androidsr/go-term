package services

import (
	"testing"
)

// MockCommandExecutor 模拟命令执行器
type MockCommandExecutor struct {
	CommandsExecuted []string
	Uploads          []string
	Downloads        []string
	SFTPCreated      bool
}

func (m *MockCommandExecutor) ExecCommand(serverID, command string) (string, error) {
	m.CommandsExecuted = append(m.CommandsExecuted, command)
	// 模拟一些命令会失败
	if command == "invalid_command" {
		return "command not found", &MockError{"Process exited with status 255"}
	}
	return "success", nil
}

func (m *MockCommandExecutor) ExecUploadFile(serverID, localPath, remotePath string) (string, error) {
	m.Uploads = append(m.Uploads, localPath+" -> "+remotePath)
	return "upload success", nil
}

func (m *MockCommandExecutor) ExecDownloadFile(serverID, remotePath, localPath string) (string, error) {
	m.Downloads = append(m.Downloads, remotePath+" -> "+localPath)
	return "download success", nil
}

func (m *MockCommandExecutor) EnsureSFTPClient(serverID string) error {
	m.SFTPCreated = true
	return nil
}

// MockError 模拟错误类型
type MockError struct {
	msg string
}

func (e *MockError) Error() string {
	return e.msg
}

func TestParseCommandsWithSpecialHandling(t *testing.T) {
	executor := NewEnhancedScriptExecutor()

	// 测试脚本内容
	scriptContent := `ls -la
invalid_command $ne
$upload /local/file.txt /remote/
$download /remote/file.txt /local/
pwd`

	parsedCommands := executor.ParseCommandsWithSpecialHandling(scriptContent)

	if len(parsedCommands) != 5 {
		t.Errorf("Expected 5 commands, got %d", len(parsedCommands))
	}

	// 检查普通命令
	if parsedCommands[0].Command != "ls -la" || parsedCommands[0].CommandType != "shell" || parsedCommands[0].ContinueOnError {
		t.Errorf("First command parsing failed")
	}

	// 检查带$ne标记的命令
	if parsedCommands[1].Command != "invalid_command" || parsedCommands[1].CommandType != "shell" || !parsedCommands[1].ContinueOnError {
		t.Errorf("Second command parsing failed: %+v", parsedCommands[1])
	}

	// 检查上传命令
	if parsedCommands[2].Command != "/local/file.txt /remote/" || parsedCommands[2].CommandType != "upload" || parsedCommands[2].ContinueOnError {
		t.Errorf("Upload command parsing failed: %+v", parsedCommands[2])
	}

	// 检查下载命令
	if parsedCommands[3].Command != "/remote/file.txt /local/" || parsedCommands[3].CommandType != "download" || parsedCommands[3].ContinueOnError {
		t.Errorf("Download command parsing failed: %+v", parsedCommands[3])
	}

	// 检查最后一个命令
	if parsedCommands[4].Command != "pwd" || parsedCommands[4].CommandType != "shell" || parsedCommands[4].ContinueOnError {
		t.Errorf("Last command parsing failed")
	}
}

func TestExecuteCommandsWithErrorHandling(t *testing.T) {
	executor := NewEnhancedScriptExecutor()
	mockExecutor := &MockCommandExecutor{}

	// 测试命令列表
	commands := []ParsedCommand{
		{Command: "ls -la", CommandType: "shell", ContinueOnError: false},
		{Command: "invalid_command", CommandType: "shell", ContinueOnError: false}, // 这个会失败并停止后续执行
		{Command: "pwd", CommandType: "shell", ContinueOnError: false},
	}

	outputs, err := executor.ExecuteCommands(commands, mockExecutor, "test-server")

	if err != nil {
		t.Errorf("ExecuteCommands should not return error: %v", err)
	}

	// 应该只执行了前两个命令，第三个被跳过不显示
	if len(outputs) != 2 {
		t.Errorf("Expected 2 outputs, got %d", len(outputs))
	}

	// 检查执行状态
	if outputs[0].Status != "success" {
		t.Errorf("First command should succeed")
	}

	if outputs[1].Status != "failed" {
		t.Errorf("Second command should fail")
	}

	// 检查详细的错误信息
	if outputs[1].Error != "Process exited with status 255\n详细输出:\ncommand not found" {
		t.Errorf("Second command should have detailed error, got: %s", outputs[1].Error)
	}
}

func TestExecuteCommandsWithContinueOnError(t *testing.T) {
	executor := NewEnhancedScriptExecutor()
	mockExecutor := &MockCommandExecutor{}

	// 测试命令列表
	commands := []ParsedCommand{
		{Command: "ls -la", CommandType: "shell", ContinueOnError: false},
		{Command: "invalid_command", CommandType: "shell", ContinueOnError: true}, // 这个会失败但继续执行
		{Command: "pwd", CommandType: "shell", ContinueOnError: false},
	}

	outputs, err := executor.ExecuteCommands(commands, mockExecutor, "test-server")

	if err != nil {
		t.Errorf("ExecuteCommands should not return error: %v", err)
	}

	// 应该执行了所有命令
	if len(outputs) != 3 {
		t.Errorf("Expected 3 outputs, got %d", len(outputs))
	}

	// 检查执行状态
	if outputs[0].Status != "success" {
		t.Errorf("First command should succeed")
	}

	if outputs[1].Status != "failed" {
		t.Errorf("Second command should fail")
	}

	// 第三个命令应该被执行而不是被跳过
	if outputs[2].Status != "success" {
		t.Errorf("Third command should be executed and succeed, got status: %s", outputs[2].Status)
	}
}

func TestHandleUploadCommand(t *testing.T) {
	executor := NewEnhancedScriptExecutor()
	mockExecutor := &MockCommandExecutor{}

	result, err := executor.handleUploadCommand(mockExecutor, "test-server", "/local/test.txt /remote/dir/")

	if err != nil {
		t.Errorf("Upload command should not fail: %v", err)
	}

	if result != "upload success" {
		t.Errorf("Expected 'upload success', got '%s'", result)
	}

	if !mockExecutor.SFTPCreated {
		t.Errorf("SFTP client should be created")
	}

	if len(mockExecutor.Uploads) != 1 {
		t.Errorf("Expected 1 upload, got %d", len(mockExecutor.Uploads))
	}

	expected := "/local/test.txt -> /remote/dir/test.txt"
	if mockExecutor.Uploads[0] != expected {
		t.Errorf("Expected '%s', got '%s'", expected, mockExecutor.Uploads[0])
	}
}

func TestHandleDownloadCommand(t *testing.T) {
	executor := NewEnhancedScriptExecutor()
	mockExecutor := &MockCommandExecutor{}

	result, err := executor.handleDownloadCommand(mockExecutor, "test-server", "/remote/test.txt /local/dir/")

	if err != nil {
		t.Errorf("Download command should not fail: %v", err)
	}

	if result != "download success" {
		t.Errorf("Expected 'download success', got '%s'", result)
	}

	if !mockExecutor.SFTPCreated {
		t.Errorf("SFTP client should be created")
	}

	if len(mockExecutor.Downloads) != 1 {
		t.Errorf("Expected 1 download, got %d", len(mockExecutor.Downloads))
	}

	expected := "/remote/test.txt -> /local/dir/"
	if mockExecutor.Downloads[0] != expected {
		t.Errorf("Expected '%s', got '%s'", expected, mockExecutor.Downloads[0])
	}
}
