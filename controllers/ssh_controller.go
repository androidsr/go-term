package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"go-term/models"
	"go-term/services"
)

// SSHController SSH控制器
type SSHController struct {
	ctx              context.Context
	serverManager    *services.ServerManager
	scriptManager    *services.ScriptManager
	scriptParser     *services.ScriptParser
	enhancedExecutor *services.EnhancedScriptExecutor
	connections      map[string]*services.SSHConnection
	sftpClients      map[string]*sftp.Client
	terminalSessions map[string]*services.TerminalSession

	// 配置文件相关
	configFile         string
	useEncryption      bool
	encryptionPassword string
	needReencrypt      bool // 标记是否需要重新加密保存

	// 全局用于保护 map 的读写（短时持有）
	mutex sync.RWMutex

	// per-server lock，用于序列化同一 server 上的高风险操作（创建/关闭 session 等）
	locksMutex     sync.Mutex
	perServerLocks map[string]*sync.Mutex
}

// NewSSHController 创建新的SSH控制器
func NewSSHController() *SSHController {
	return &SSHController{
		connections:      make(map[string]*services.SSHConnection),
		sftpClients:      make(map[string]*sftp.Client),
		terminalSessions: make(map[string]*services.TerminalSession),
		perServerLocks:   make(map[string]*sync.Mutex),
		configFile:       "config/servers.dat", // 默认使用加密文件扩展名
		useEncryption:    true,                 // 默认启用加密
		needReencrypt:    false,                // 默认不需要重新加密
		scriptManager:    services.NewScriptManager(),
		scriptParser:     services.NewScriptParser(),
		enhancedExecutor: services.NewEnhancedScriptExecutor(),
	}
}

// SetEncryptionConfig 设置加密配置
func (sc *SSHController) SetEncryptionConfig(useEncryption bool, password string) {
	sc.useEncryption = useEncryption
	sc.encryptionPassword = password

	// 根据是否使用加密设置配置文件路径
	if useEncryption {
		sc.configFile = "config/servers.dat"
	} else {
		sc.configFile = "config/servers.json"
	}
}

// helper: 获取或创建单个 server 的互斥锁
func (sc *SSHController) getServerLock(serverID string) *sync.Mutex {
	sc.locksMutex.Lock()
	defer sc.locksMutex.Unlock()
	if l, ok := sc.perServerLocks[serverID]; ok {
		return l
	}
	l := &sync.Mutex{}
	sc.perServerLocks[serverID] = l
	return l
}

// Startup 初始化控制器
func (sc *SSHController) Startup(ctx context.Context) {
	sc.ctx = ctx
	sc.serverManager = services.NewServerManager()

	// 加载服务器配置
	if sc.useEncryption {
		// 使用新的加载方法，支持从明文自动转换为加密格式
		needReencrypt, err := sc.serverManager.LoadFromFileWithFallback(sc.configFile, sc.encryptionPassword)
		if err != nil {
			fmt.Printf("警告: 无法加载服务器配置: %v\n", err)
		}
		sc.needReencrypt = needReencrypt
	} else {
		if err := sc.serverManager.LoadFromFile(sc.configFile); err != nil {
			fmt.Printf("警告: 无法加载服务器配置: %v\n", err)
		}
	}

	// 如果需要重新加密（从明文加载），则保存为加密格式
	if sc.needReencrypt && sc.useEncryption {
		if err := sc.saveConfig(); err != nil {
			fmt.Printf("警告: 无法保存加密配置: %v\n", err)
		} else {
			fmt.Println("配置文件已从明文格式转换为加密格式")
			sc.needReencrypt = false
		}
	}

	// 加载脚本配置
	if err := sc.scriptManager.LoadFromFile("config/scripts.json"); err != nil {
		fmt.Printf("警告: 无法加载脚本配置: %v\n", err)
	}
}

// saveConfig 保存配置的辅助函数
func (sc *SSHController) saveConfig() error {
	if sc.useEncryption {
		return sc.serverManager.SaveToEncryptedFile(sc.configFile, sc.encryptionPassword)
	}
	return sc.serverManager.SaveToFile(sc.configFile)
}

// GetServerGroups 获取所有服务器分组
func (sc *SSHController) GetServerGroups() []models.ServerGroup {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	return sc.serverManager.GetGroups()
}

// GetServerConnectionStatus 获取服务器连接状态
func (sc *SSHController) GetServerConnectionStatus() map[string]bool {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	status := make(map[string]bool)

	for serverID, conn := range sc.connections {
		if conn != nil && conn.Client != nil {
			// 更可靠的检查方式：使用 SendRequest 而不是创建新 session
			// SendRequest "keepalive@openssh.com" 是轻量级检查，不会创建新 session
			_, _, err := conn.Client.SendRequest("keepalive@openssh.com", true, nil)
			if err == nil {
				status[serverID] = true
			} else {
				// 连接已断开，清理
				delete(sc.connections, serverID)
				status[serverID] = false
			}
		} else {
			status[serverID] = false
		}
	}

	return status
}

// AddServerGroup 添加服务器分组
func (sc *SSHController) AddServerGroup(group models.ServerGroup) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	sc.serverManager.AddGroup(group)

	// 保存到文件
	return sc.saveConfig()
}

// UpdateServerGroup 更新服务器分组
func (sc *SSHController) UpdateServerGroup(group models.ServerGroup) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	err := sc.serverManager.UpdateGroup(group)
	if err != nil {
		return err
	}

	// 保存到文件
	return sc.saveConfig()
}

// DeleteServerGroup 删除服务器分组
func (sc *SSHController) DeleteServerGroup(groupID string) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	err := sc.serverManager.DeleteGroup(groupID)
	if err != nil {
		return err
	}

	// 保存到文件
	return sc.saveConfig()
}

// AddServer 添加服务器
func (sc *SSHController) AddServer(groupID string, server models.Server) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	err := sc.serverManager.AddServer(groupID, server)
	if err != nil {
		return err
	}

	// 保存到文件
	return sc.saveConfig()
}

// UpdateServer 更新服务器
func (sc *SSHController) UpdateServer(groupID string, server models.Server) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	err := sc.serverManager.UpdateServer(groupID, server)
	if err != nil {
		return err
	}

	// 保存到文件
	return sc.saveConfig()
}

// DeleteServer 删除服务器
func (sc *SSHController) DeleteServer(groupID, serverID string) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	err := sc.serverManager.DeleteServer(groupID, serverID)
	if err != nil {
		return err
	}

	// 保存到文件
	return sc.saveConfig()
}

// ConnectToServer 连接到服务器
func (sc *SSHController) ConnectToServer(serverID string) (string, error) {
	// 先读取服务器配置 & 当前连接状态（短锁）
	sc.mutex.RLock()
	_, already := sc.connections[serverID]
	sc.mutex.RUnlock()

	if already {
		return "已连接到服务器", nil
	}

	// 从 serverManager 获取 server 信息（此处使用方法可能会读取内部数据结构；serverManager 本身应保证并发安全）
	server, err := sc.serverManager.GetServerByID(serverID)
	if err != nil {
		return "", fmt.Errorf("无法找到服务器: %v", err)
	}

	// 创建连接是在无全局锁下进行的耗时 IO
	connection := &services.SSHConnection{}
	if err := connection.Connect(server.Host, server.Port, server.Username, server.Password, server.KeyFile); err != nil {
		return "", fmt.Errorf("连接失败: %v", err)
	}

	// 成功后将连接写入 map（短锁）
	sc.mutex.Lock()
	// double-check 避免竞态：可能在我们创建期间别人已创建
	if existing, ok := sc.connections[serverID]; ok && existing.Client != nil {
		// 我们的 connection 多余，先 close 掉自己（如果实现需要）
		sc.mutex.Unlock()
		// 尝试关闭新创建的 connection 以释放资源（忽略返回错误）
		connection.Close()
		return "已连接到服务器", nil
	}
	sc.connections[serverID] = connection
	sc.mutex.Unlock()

	return "连接成功", nil
}

// ExecuteCommand 在服务器上执行命令
func (sc *SSHController) ExecuteCommand(serverID, command string) (string, error) {
	// 优先检查是否存在终端会话（短锁）
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if hasSession {
		// 在发送命令前确保shell状态干净
		// 发送 Ctrl+U 清除当前可能存在的输入，然后发送用户选择的命令
		session.SendCommandWithoutNewline("\x15") // Ctrl+U: 清除当前行
		time.Sleep(5 * time.Millisecond)

		// 清空输出缓冲区，清除之前补全操作留下的临时数据
		session.ClearOutputBuffer()

		// 通过终端会话发送命令（session.SendCommand 可能是非阻塞或有独立超时）
		if err := session.SendCommand(command); err != nil {
			return "", fmt.Errorf("发送命令失败: %v", err)
		}
		return "命令已发送", nil
	}

	// 否则直接通过 SSHConnection 执行（读取 connection 副本，不持锁做耗时）
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}

	result, err := conn.ExecuteCommand(command)
	if err != nil {
		return "", fmt.Errorf("执行命令失败: %v", err)
	}
	return result, nil
}

// DisconnectFromServer 断开服务器连接 - 修复死锁版本
func (sc *SSHController) DisconnectFromServer(serverID string) (string, error) {
	// 使用超时上下文避免死锁
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// 分步操作，避免锁嵌套
	
	// 1. 先获取连接信息（只读）
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[serverID]
	conn, hasConn := sc.connections[serverID]
	sftpClient, hasSftp := sc.sftpClients[serverID]
	sc.mutex.RUnlock()
	
	var errMsgs []string
	
	// 2. 在无锁状态下关闭资源
	if hasSession && session != nil {
		if err := sc.closeSessionWithTimeout(ctx, session); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("关闭终端会话失败: %v", err))
		}
	}
	
	if hasSftp && sftpClient != nil {
		if err := sftpClient.Close(); err != nil {
			log.Printf("关闭SFTP客户端警告: %v", err)
		}
	}
	
	if hasConn && conn != nil {
		conn.Close()
	}
	
	// 3. 最后清理数据结构
	sc.mutex.Lock()
	if hasSession {
		delete(sc.terminalSessions, serverID)
	}
	if hasSftp {
		delete(sc.sftpClients, serverID)
	}
	if hasConn {
		delete(sc.connections, serverID)
	}
	sc.mutex.Unlock()
	
	// 清理per-server锁
	sc.locksMutex.Lock()
	delete(sc.perServerLocks, serverID)
	sc.locksMutex.Unlock()
	
	if len(errMsgs) > 0 {
		return "", fmt.Errorf("断开连接时发生错误: %s", strings.Join(errMsgs, "; "))
	}
	
	return "服务器连接已安全断开", nil
}

// closeSessionWithTimeout 带超时的会话关闭
func (sc *SSHController) closeSessionWithTimeout(ctx context.Context, session *services.TerminalSession) error {
	resultChan := make(chan error, 1)
	
	go func() {
		resultChan <- session.Close()
	}()
	
	select {
	case err := <-resultChan:
		if err != nil && err != io.EOF {
			return err
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("关闭会话超时")
	}
}

// IsTerminalSessionActive 检查终端会话是否仍然活跃
func (sc *SSHController) IsTerminalSessionActive(serverID string) bool {
	sc.mutex.RLock()
	session, exists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !exists {
		return false
	}

	return sc.isSessionActive(session)
}

// isConnectionHealthy 检查连接健康状态
func (sc *SSHController) isConnectionHealthy(serverID string) bool {
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sc.mutex.RUnlock()
	
	if !exists || conn == nil || conn.Client == nil {
		return false
	}
	
	// 简单的连通性检查
	_, err := conn.Client.NewSession()
	if err != nil {
		// 连接已断开，清理
		sc.mutex.Lock()
		delete(sc.connections, serverID)
		sc.mutex.Unlock()
		return false
	}
	
	return true
}

// isSessionActive 检查会话是否真正活跃
func (sc *SSHController) isSessionActive(session *services.TerminalSession) bool {
	if session == nil || session.OutputChan == nil {
		return false
	}
	
	select {
	case _, ok := <-session.OutputChan:
		return ok // 如果channel已关闭，返回false
	default:
		// channel正常，尝试发送一个简单的心跳命令
		// 这里可以添加更复杂的健康检查逻辑
		return true
	}
}

// CreateTerminalSession 创建终端会话 - 修复竞态条件
func (sc *SSHController) CreateTerminalSession(serverID string) (string, error) {
	// 1. 检查连接状态
	if !sc.isConnectionHealthy(serverID) {
		return "", fmt.Errorf("服务器连接无效，请重新连接")
	}
	
	// 2. 检查现有会话（使用更严格的检查）
	sc.mutex.RLock()
	existingSession, exists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()
	
	if exists && existingSession != nil {
		// 验证会话是否真的有效
		if sc.isSessionActive(existingSession) {
			return "终端会话已存在且活跃", nil
		}
		
		// 清理无效会话
		sc.mutex.Lock()
		delete(sc.terminalSessions, serverID)
		sc.mutex.Unlock()
	}
	
	// 3. 使用无锁方式创建会话
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sc.mutex.RUnlock()
	
	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接")
	}
	
	// 创建会话（耗时操作，不持锁）
	terminalSession, err := conn.CreateTerminalSession(80, 24)
	if err != nil {
		return "", fmt.Errorf("创建终端会话失败: %v", err)
	}
	
	// 4. 原子性存储新会话
	sc.mutex.Lock()
	// 最终检查，避免重复创建
	if _, exists := sc.terminalSessions[serverID]; exists {
		sc.mutex.Unlock()
		terminalSession.Close() // 清理多余的会话
		return "终端会话已存在", nil
	}
	sc.terminalSessions[serverID] = terminalSession
	sc.mutex.Unlock()

	// 设置事件推送函数并启动推送协程
	terminalSession.SetEventEmitter(serverID, func(event string, data ...interface{}) {
		runtime.EventsEmit(sc.ctx, event, data...)
	})
	terminalSession.StartOutputPusher()

	return "终端会话创建成功", nil
}

// CreateTerminalSessionWithSize 创建指定尺寸的终端会话
func (sc *SSHController) CreateTerminalSessionWithSize(serverID string, width, height int) (string, error) {
	// 先短锁读取 connection 和会话存在性
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	_, sessionExists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}

	// 检查现有会话是否有效
	if sessionExists {
		// 检查会话是否仍然活跃
		if !sc.IsTerminalSessionActive(serverID) {
			fmt.Println("会话已失效", serverID)
			// 会话已失效，清理并允许创建新会话
			sc.mutex.Lock()
			delete(sc.terminalSessions, serverID)
			sc.mutex.Unlock()
		} else {
			// 会话仍然有效
			return "终端会话已存在", nil
		}
	}

	// 使用 per-server lock 序列化本服务器的 create/close 操作
	serverLock := sc.getServerLock(serverID)
	serverLock.Lock()
	defer serverLock.Unlock()

	// createTerminal 是耗时 IO —— 必须在没有持有全局 sc.mutex 的情况下执行
	terminalSession, err := conn.CreateTerminalSession(width, height)
	if err != nil {
		return "", fmt.Errorf("创建终端会话失败: %v", err)
	}

	// 创建成功后用短锁写回 map
	sc.mutex.Lock()
	// 再次检查（double-check）避免竞态：在我们创建期间别人可能已创建
	if _, ok := sc.terminalSessions[serverID]; ok {
		// 已有会话：关闭我们刚创建的会话并返回已存在
		sc.mutex.Unlock()
		_ = terminalSession.Close()
		return "终端会话已存在", nil
	}
	sc.terminalSessions[serverID] = terminalSession
	sc.mutex.Unlock()

	// 设置事件推送函数并启动推送协程
	terminalSession.SetEventEmitter(serverID, func(event string, data ...interface{}) {
		runtime.EventsEmit(sc.ctx, event, data...)
	})
	terminalSession.StartOutputPusher()

	return "终端会话创建成功", nil
}

// CreateSFTPClient 创建SFTP客户端
func (sc *SSHController) CreateSFTPClient(serverID string) (string, error) {
	// 读取 connection 副本（短锁）
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	_, sftpExists := sc.sftpClients[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}
	if sftpExists {
		return "SFTP客户端已存在", nil
	}

	// 也序列化同一 server 的 sftp create/close
	serverLock := sc.getServerLock(serverID)
	serverLock.Lock()
	defer serverLock.Unlock()

	// 耗时 IO：创建 sftp client
	sftpClient, err := conn.CreateSFTPClient()
	if err != nil {
		return "", fmt.Errorf("创建SFTP客户端失败: %v", err)
	}

	// 写回 map（短锁）
	sc.mutex.Lock()
	// double-check
	if _, ok := sc.sftpClients[serverID]; ok {
		sc.mutex.Unlock()
		_ = sftpClient.Close()
		return "SFTP客户端已存在", nil
	}
	sc.sftpClients[serverID] = sftpClient
	sc.mutex.Unlock()

	return "SFTP客户端创建成功", nil
}

// ReadTerminalOutput 读取终端输出
func (sc *SSHController) ReadTerminalOutput(serverID string) (string, error) {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("终端会话不存在")
	}

	select {
	case out, ok := <-terminalSession.OutputChan:
		if !ok {
			return "", fmt.Errorf("终端输出已关闭")
		}
		return string(out), nil
	default:
		return "", nil // 没有新数据时立即返回，不阻塞
	}
}

// GetTerminalLastOutput 获取终端最后的输出内容
func (sc *SSHController) GetTerminalLastOutput(serverID string) (string, error) {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("终端会话不存在")
	}

	return terminalSession.GetLastOutput(), nil
}

// ClearTerminalOutputBuffer 清空终端输出缓冲区
func (sc *SSHController) ClearTerminalOutputBuffer(serverID string) error {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("终端会话不存在")
	}

	terminalSession.ClearOutputBuffer()
	return nil
}

// GetAutoCompleteSuggestions 获取自动补全建议
func (sc *SSHController) GetAutoCompleteSuggestions(serverID, partialCommand string) ([]string, error) {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("终端会话不存在")
	}

	// 清空输出缓冲区
	terminalSession.ClearOutputBuffer()

	// 发送部分命令（不带换行符）
	if err := terminalSession.SendCommandWithoutNewline(partialCommand); err != nil {
		return nil, fmt.Errorf("发送命令失败: %v", err)
	}

	// 等待一小段时间让shell处理
	time.Sleep(20 * time.Millisecond)

	// 发送两次Tab字符获取补全选项列表
	if err := terminalSession.SendCommandWithoutNewline("\t\t"); err != nil {
		return nil, fmt.Errorf("发送Tab失败: %v", err)
	}

	// 等待shell处理补全
	time.Sleep(150 * time.Millisecond)

	// 获取补全输出
	output := terminalSession.GetLastOutput()

	// 如果没有获取到有效的补全输出，尝试单次Tab
	if strings.TrimSpace(output) == "" || len(strings.TrimSpace(output)) < 2 {
		// 再次清空缓冲区
		terminalSession.ClearOutputBuffer()

		// 重新发送命令
		if err := terminalSession.SendCommandWithoutNewline(partialCommand); err != nil {
			return nil, fmt.Errorf("重新发送命令失败: %v", err)
		}
		time.Sleep(20 * time.Millisecond)

		// 发送单次Tab
		if err := terminalSession.SendCommandWithoutNewline("\t"); err != nil {
			return nil, fmt.Errorf("发送单次Tab失败: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		// 获取新的输出
		output = terminalSession.GetLastOutput()
	}

	// 解析补全建议
	suggestions := terminalSession.ParseAutoCompleteSuggestions(partialCommand, output)

	// 只清空内部缓冲区，不在终端发送任何清理字符
	// 前端会负责显示管理，避免污染终端状态
	terminalSession.ClearOutputBuffer()

	return suggestions, nil
}

// UploadFile 上传文件
func (sc *SSHController) UploadFile(serverID, localPath, remotePath string) (string, error) {
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sftpClient, sftpExists := sc.sftpClients[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}
	if !sftpExists {
		return "", fmt.Errorf("SFTP客户端未创建，请先创建SFTP客户端")
	}

	// 上传文件（不持锁）
	if err := conn.UploadFile(sftpClient, localPath, remotePath); err != nil {
		return "", fmt.Errorf("上传文件失败: %v", err)
	}
	return "文件上传成功", nil
}

// DownloadFile 下载文件
func (sc *SSHController) DownloadFile(serverID, remotePath, localPath string) (string, error) {
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sftpClient, sftpExists := sc.sftpClients[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}
	if !sftpExists {
		return "", fmt.Errorf("SFTP客户端未创建，请先创建SFTP客户端")
	}

	// 下载文件（不持锁）
	if err := conn.DownloadFile(sftpClient, remotePath, localPath); err != nil {
		return "", fmt.Errorf("下载文件失败: %v", err)
	}
	return "文件下载成功", nil
}

// ListDirectory 列出目录内容
func (sc *SSHController) ListDirectory(serverID, path string) ([]services.FileInfo, error) {
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sftpClient, sftpExists := sc.sftpClients[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return nil, fmt.Errorf("服务器未连接，请先连接服务器")
	}
	if !sftpExists {
		return nil, fmt.Errorf("SFTP客户端未创建，请先创建SFTP客户端")
	}

	// 列出目录内容（不持锁）
	files, err := conn.ListDirectory(sftpClient, path)
	if err != nil {
		return nil, fmt.Errorf("列出目录内容失败: %v", err)
	}
	return files, nil
}

// CreateDirectory 创建目录
func (sc *SSHController) CreateDirectory(serverID, path string) (string, error) {
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sftpClient, sftpExists := sc.sftpClients[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}
	if !sftpExists {
		return "", fmt.Errorf("SFTP客户端未创建，请先创建SFTP客户端")
	}

	// 创建目录（不持锁）
	if err := conn.CreateDirectory(sftpClient, path); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}
	return "目录创建成功", nil
}

// DeleteFile 删除文件或目录
func (sc *SSHController) DeleteFile(serverID, path string) (string, error) {
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sftpClient, sftpExists := sc.sftpClients[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}
	if !sftpExists {
		return "", fmt.Errorf("SFTP客户端未创建，请先创建SFTP客户端")
	}

	// 删除文件或目录（不持锁）
	if err := conn.DeleteFile(sftpClient, path); err != nil {
		return "", fmt.Errorf("删除文件失败: %v", err)
	}
	return "文件删除成功", nil
}

// ExecuteCommandWithoutNewline 执行命令但不添加换行符
func (sc *SSHController) ExecuteCommandWithoutNewline(serverID, command string) (string, error) {
	// 优先检查是否存在终端会话（短锁）
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if hasSession {
		// 通过终端会话发送命令（不添加换行符）
		if err := session.SendCommandWithoutNewline(command); err != nil {
			return "", fmt.Errorf("发送命令失败: %v", err)
		}
		return "命令已发送", nil
	}

	return "", fmt.Errorf("终端会话不存在")
}

// InterruptCommand 中断当前正在执行的命令（发送 Ctrl+C）
func (sc *SSHController) InterruptCommand(serverID string) (string, error) {
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !hasSession {
		return "", fmt.Errorf("终端会话不存在")
	}

	// 发送多次 Ctrl+C 确保中断信号能够发送
	// 在高输出场景下，一次可能不够
	for i := 0; i < 3; i++ {
		if err := session.SendCommandWithoutNewline("\x03"); err != nil {
			return "", fmt.Errorf("发送中断信号失败: %v", err)
		}
		// 短暂延迟，确保信号被处理
		time.Sleep(10 * time.Millisecond)
	}

	return "命令已中断", nil
}

// CloseTerminalSession 关闭指定的终端会话
func (sc *SSHController) CloseTerminalSession(serverID string) (string, error) {
	// 序列化同 server 的操作
	serverLock := sc.getServerLock(serverID)
	serverLock.Lock()
	defer serverLock.Unlock() // 使用标准的defer方式确保锁释放
	// 读取会话副本（短锁），然后释放锁进行关闭
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !hasSession {
		return "终端会话不存在", nil
	}

	var errMsg string

	// 使用更严格的超时控制
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	fmt.Println("会话副本读取完成", serverID)

	closeChan := make(chan error, 1)
	go func() {
		closeChan <- session.Close()
	}()

	select {
	case err := <-closeChan:
		// EOF错误在连接已断开时是正常的，不需要报告为错误
		if err != nil && err != io.EOF {
			errMsg = fmt.Sprintf("关闭终端会话时出错: %v", err)
			log.Printf("关闭终端会话时出错: %v", err)
		} else if err == io.EOF {
			log.Printf("终端会话已断开连接: %v", serverID)
		}
	case <-ctx.Done():
		errMsg = "关闭终端会话超时"
		log.Printf("关闭终端会话超时，强制终止")
		// 在超时情况下，尝试强制清理资源
	}

	// 确保清理数据结构（短锁）
	sc.mutex.Lock()
	delete(sc.terminalSessions, serverID)
	sc.mutex.Unlock()

	if errMsg != "" {
		return "", fmt.Errorf("%s", errMsg)
	}
	return "终端会话已关闭", nil
}

// ResizeTerminal 调整终端大小
func (sc *SSHController) ResizeTerminal(serverID string, width, height int) (string, error) {
	// 读取终端会话（短锁）
	sc.mutex.RLock()
	session, exists := sc.terminalSessions[serverID]
	sc.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("终端会话不存在")
	}

	// 调整终端大小
	if err := session.ResizeTerminal(width, height); err != nil {
		return "", fmt.Errorf("调整终端大小失败: %v", err)
	}

	return "终端大小调整成功", nil
}

// ========== 脚本管理相关方法 ==========

// GetBatchScripts 获取所有批量脚本
func (sc *SSHController) GetBatchScripts() []models.BatchScript {
	return sc.scriptManager.GetScripts()
}

// AddBatchScript 添加批量脚本
func (sc *SSHController) AddBatchScript(script models.BatchScript) error {
	return sc.scriptManager.AddScript(script)
}

// UpdateBatchScript 更新批量脚本
func (sc *SSHController) UpdateBatchScript(script models.BatchScript) error {
	return sc.scriptManager.UpdateScript(script)
}

// DeleteBatchScript 删除批量脚本
func (sc *SSHController) DeleteBatchScript(scriptID string) error {
	return sc.scriptManager.DeleteScript(scriptID)
}

// ExecuteBatchScript 执行批量脚本
func (sc *SSHController) ExecuteBatchScript(scriptID string) (map[string]models.ScriptExecution, error) {
	// 获取脚本
	script, err := sc.scriptManager.GetScriptByID(scriptID)
	if err != nil {
		return nil, fmt.Errorf("获取脚本失败: %v", err)
	}

	// 获取所有服务器组以解析服务器名称
	groups := sc.serverManager.GetGroups()
	serverMap := make(map[string]string)
	for _, group := range groups {
		for _, server := range group.Servers {
			serverMap[server.ID] = server.Name
		}
	}

// 并发执行脚本 - 添加并发控制
	results := make(map[string]models.ScriptExecution)
	var wg sync.WaitGroup
	var resultMutex sync.Mutex
	
	// 并发控制 - 限制最大并发数为10
	maxConcurrent := 10
	semaphore := make(chan struct{}, maxConcurrent)

	for _, serverID := range script.ServerIDs {
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()
			
			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			execution := models.ScriptExecution{
				ID:             fmt.Sprintf("exec_%s_%s_%d", scriptID, sid, time.Now().Unix()),
				ScriptID:       scriptID,
				ServerID:       sid,
				ServerName:     serverMap[sid],
				Status:         "pending",
				StartTime:      time.Now().Format("2006-01-02 15:04:05"),
				CommandOutputs: make([]models.CommandOutput, 0),
			}

			resultMutex.Lock()
			results[sid] = execution
			resultMutex.Unlock()

			var commandOutputs []models.CommandOutput
			var execErr error

			// 根据执行类型选择执行方式
			if script.ExecutionType == "script" {
				// 脚本模式：将整个脚本内容作为一个整体执行
				commandOutputs, execErr = sc.enhancedExecutor.ExecuteScriptMode(script.Content, sc, sid)
			} else {
				// 命令模式：逐条执行每个命令（默认模式）
				parsedCommands := sc.enhancedExecutor.ParseCommands(script.Content)
				if len(parsedCommands) == 0 {
					execErr = fmt.Errorf("脚本中没有有效的命令")
				} else {
					commandOutputs, execErr = sc.enhancedExecutor.ExecuteCommandMode(parsedCommands, sc, sid)
				}
			}

			execution.EndTime = time.Now().Format("2006-01-02 15:04:05")
			execution.CommandOutputs = commandOutputs

			// 检查是否有失败的命令
			hasFailedCommand := false
			for _, cmdOutput := range commandOutputs {
				if cmdOutput.Status == "failed" {
					hasFailedCommand = true
					break
				}
			}

			// 根据执行结果设置状态
			if execErr != nil {
				execution.Status = "failed"
				execution.Error = fmt.Sprintf("执行错误: %v", execErr)
			} else if hasFailedCommand {
				execution.Status = "failed"
				// 显示第一个失败的命令的错误信息
				for _, cmdOutput := range commandOutputs {
					if cmdOutput.Status == "failed" {
						// 优先使用命令级别的错误信息
						if cmdOutput.Error != "" {
							execution.Error = cmdOutput.Error
						} else if cmdOutput.Output != "" {
							execution.Error = cmdOutput.Output
						} else {
							execution.Error = "命令执行失败，但没有详细的错误信息"
						}
						break
					}
				}
				// 如果没有找到具体的错误信息，设置默认错误
				if execution.Error == "" {
					execution.Error = "脚本执行过程中发生了未知的错误"
				}
			} else {
				execution.Status = "success"
			}

			// 最终检查：确保失败状态一定有错误信息
			if execution.Status == "failed" && execution.Error == "" {
				execution.Error = "执行失败，但未能获取具体的错误信息"
			}

			// 确保命令输出也被正确设置
			if execution.Status == "failed" && len(commandOutputs) > 0 {
				// 检查最后一个命令是否失败
				lastCmd := commandOutputs[len(commandOutputs)-1]
				if lastCmd.Status == "failed" {
					// 确保主执行对象也有错误输出
					if execution.Output == "" && lastCmd.Output != "" {
						execution.Output = lastCmd.Output
					}
					if execution.Error == "" && lastCmd.Error != "" {
						execution.Error = lastCmd.Error
					}
				}
			}

			resultMutex.Lock()
			results[sid] = execution
			resultMutex.Unlock()
		}(serverID)
	}

	wg.Wait()
	return results, nil
}

// SendScriptToTerminal 逐行发送脚本命令到终端（用于命令模式）
// wails:export
func (sc *SSHController) SendScriptToTerminal(scriptID string, serverID string) error {
	// 获取脚本
	script, err := sc.scriptManager.GetScriptByID(scriptID)
	if err != nil {
		return fmt.Errorf("获取脚本失败: %v", err)
	}

	// 检查服务器是否在脚本的目标服务器列表中
	found := false
	for _, sid := range script.ServerIDs {
		if sid == serverID {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("服务器不在脚本的目标服务器列表中")
	}

	// 只处理命令模式的脚本
	if script.ExecutionType != "command" {
		return fmt.Errorf("仅支持命令模式脚本的终端交互执行")
	}

	// 解析命令
	parsedCommands := sc.enhancedExecutor.ParseCommands(script.Content)
	if len(parsedCommands) == 0 {
		return fmt.Errorf("脚本中没有有效的命令")
	}

	// 确保终端会话存在
	_, err = sc.CreateTerminalSession(serverID)
	if err != nil {
		return fmt.Errorf("创建终端会话失败: %v", err)
	}

	// 逐行发送命令到终端
	for _, parsedCmd := range parsedCommands {
		// 处理文件上传命令
		if parsedCmd.CommandType == "upload" {
			// 解析上传命令参数
			parts := strings.Fields(parsedCmd.Command)
			if len(parts) >= 2 {
				localPath := parts[0]
				remoteDir := parts[1]

				// 构造远程文件路径
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

				// 在终端中显示上传信息
				// 注释掉下面的代码，避免在终端中输出上传日志
				// _, sendErr := sc.ExecuteCommandWithoutNewline(serverID, fmt.Sprintf("echo \"正在上传文件: %s -> %s\"\n", localPath, remotePath))
				// if sendErr != nil {
				// 	fmt.Printf("发送信息到终端失败: %v\n", sendErr)
				// }

				// 确保SFTP客户端已创建
				err := sc.EnsureSFTPClient(serverID)
				if err != nil {
					fmt.Printf("创建SFTP客户端失败: %v\n", err)
					continue
				}

				// 执行上传操作并等待完成
				_, err = sc.UploadFile(serverID, localPath, remotePath)
				if err != nil {
					fmt.Printf("文件上传失败: %v\n", err)
				} else {
					fmt.Printf("文件上传成功: %s -> %s\n", localPath, remotePath)
				}
			} else {
				// 发送错误信息到终端
				// 注释掉下面的代码，避免在终端中输出上传日志
				// _, sendErr := sc.ExecuteCommandWithoutNewline(serverID, fmt.Sprintf("echo \"上传命令格式错误: %s\"\n", parsedCmd.Command))
				// if sendErr != nil {
				// 	fmt.Printf("发送错误信息到终端失败: %v\n", sendErr)
				// }
			}
			// 添加一个小延迟
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// 处理文件下载命令
		if parsedCmd.CommandType == "download" {
			// 解析下载命令参数
			parts := strings.Fields(parsedCmd.Command)
			if len(parts) >= 2 {
				remotePath := parts[0]
				localPath := parts[1]

				// 在终端中显示下载信息
				// 注释掉下面的代码，避免在终端中输出下载日志
				// _, sendErr := sc.ExecuteCommandWithoutNewline(serverID, fmt.Sprintf("echo \"正在下载文件: %s -> %s\"\n", remotePath, localPath))
				// if sendErr != nil {
				// 	fmt.Printf("发送信息到终端失败: %v\n", sendErr)
				// }

				// 确保SFTP客户端已创建
				err := sc.EnsureSFTPClient(serverID)
				if err != nil {
					fmt.Printf("创建SFTP客户端失败: %v\n", err)
					continue
				}

				// 执行下载操作并等待完成
				_, err = sc.DownloadFile(serverID, remotePath, localPath)
				if err != nil {
					fmt.Printf("文件下载失败: %v\n", err)
				} else {
					fmt.Printf("文件下载成功: %s -> %s\n", remotePath, localPath)
				}
			} else {
				// 发送错误信息到终端
				// 注释掉下面的代码，避免在终端中输出下载日志
				// _, sendErr := sc.ExecuteCommandWithoutNewline(serverID, fmt.Sprintf("echo \"下载命令格式错误: %s\"\n", parsedCmd.Command))
				// if sendErr != nil {
				// 	fmt.Printf("发送错误信息到终端失败: %v\n", sendErr)
				// }
			}
			// 添加一个小延迟
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// 处理shell类型的命令，发送到终端
		if parsedCmd.CommandType == "shell" {
			// 发送命令到终端（带换行符，让命令执行）
			_, err = sc.ExecuteCommand(serverID, parsedCmd.Command)
			if err != nil {
				// 记录错误但继续执行下一个命令
				fmt.Printf("发送命令到终端失败: %v\n", err)
			}

			// 添加一个小延迟，让用户看到命令输入的过程
			time.Sleep(500 * time.Millisecond)
		}
	}

	return nil
}

// 实现CommandExecutor接口的方法（添加Exec前缀以避免命名冲突）
func (sc *SSHController) ExecCommand(serverID, command string) (string, error) {
	return sc.ExecuteCommand(serverID, command)
}

func (sc *SSHController) ExecCommandDirect(serverID, command string) (string, error) {
	// 直接通过 SSHConnection 执行，不检查终端会话
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}

	result, err := conn.ExecuteCommand(command)
	if err != nil {
		// 如果有输出结果，说明命令执行了但有错误，返回完整的错误信息
		if result != "" {
			return result, fmt.Errorf("执行命令失败: %v\n输出: %s", err, result)
		}
		return "", fmt.Errorf("执行命令失败: %v", err)
	}
	return result, nil
}

func (sc *SSHController) ExecCommandsInSharedSession(serverID string, commands []string) ([]string, error) {
	// 直接通过 SSHConnection 执行，不检查终端会话
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return nil, fmt.Errorf("服务器未连接，请先连接服务器")
	}

	result, err := conn.ExecuteCommandsWithSharedSession(commands)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (sc *SSHController) ExecUploadFile(serverID, localPath, remotePath string) (string, error) {
	return sc.UploadFile(serverID, localPath, remotePath)
}

func (sc *SSHController) ExecDownloadFile(serverID, remotePath, localPath string) (string, error) {
	return sc.DownloadFile(serverID, remotePath, localPath)
}

// HandleFileUploadRequest 处理文件上传请求
func (sc *SSHController) HandleFileUploadRequest(serverID, localPath, remotePath string) error {
	// 确保SFTP客户端已创建
	err := sc.EnsureSFTPClient(serverID)
	if err != nil {
		return fmt.Errorf("创建SFTP客户端失败: %v", err)
	}

	// 执行上传操作并等待完成
	_, err = sc.UploadFile(serverID, localPath, remotePath)
	if err != nil {
		return fmt.Errorf("文件上传失败: %v", err)
	}

	return nil
}

// HandleFileDownloadRequest 处理文件下载请求
func (sc *SSHController) HandleFileDownloadRequest(serverID, remotePath, localPath string) error {
	// 确保SFTP客户端已创建
	err := sc.EnsureSFTPClient(serverID)
	if err != nil {
		return fmt.Errorf("创建SFTP客户端失败: %v", err)
	}

	// 执行下载操作并等待完成
	_, err = sc.DownloadFile(serverID, remotePath, localPath)
	if err != nil {
		return fmt.Errorf("文件下载失败: %v", err)
	}

	return nil
}

// EnsureSFTPClient 确保SFTP客户端已创建
func (sc *SSHController) EnsureSFTPClient(serverID string) error {
	// 检查SFTP客户端是否已存在
	sc.mutex.RLock()
	_, sftpExists := sc.sftpClients[serverID]
	sc.mutex.RUnlock()

	if sftpExists {
		return nil
	}

	// 创建SFTP客户端
	_, err := sc.CreateSFTPClient(serverID)
	return err
}
