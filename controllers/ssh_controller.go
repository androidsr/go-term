package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v3/pkg/application"

	"go-term/models"
	"go-term/services"
)

// SSHController SSH控制器
type SSHController struct {
	app              *application.App
	serverManager    *services.ServerManager
	scriptManager    *services.ScriptManager
	scriptParser     *services.ScriptParser
	enhancedExecutor *services.EnhancedScriptExecutor
	connections      map[string]*services.SSHConnection
	sftpClients      map[string]*sftp.Client
	// terminalSessions 以独立的 sessionID 为键，支持同一服务器多个终端会话
	terminalSessions map[string]*services.TerminalSession
	// sessionServer 记录 sessionID -> serverID 的映射
	sessionServer map[string]string

	// 配置文件相关
	configFile         string
	useEncryption      bool
	encryptionPassword string
	needReencrypt      bool // 标记是否需要重新加密保存

	// 运维配置 & 上次打开路径
	opsConfigManager *services.OpsConfigManager
	lastPaths        *services.LastPathStore

	// 命令历史 / 片段库 / 端口转发
	commandHistory  *services.CommandHistoryStore
	snippetManager  *services.SnippetManager
	portForwardMgr  *services.PortForwardManager
	portForwardCfgs map[string][]services.PortForward // serverID -> 转发配置（持久化，连接后自动恢复）
	pfMutex         sync.Mutex

	// 自动重连
	reconnectMu    sync.Mutex
	reconnectState map[string]*reconnectInfo
	autoReconnect  map[string]bool // serverID -> 是否启用自动重连（持久化）

	// 全局用于保护 map 的读写（短时持有）
	mutex sync.RWMutex

	// per-server lock，用于序列化同一 server 上的高风险操作（创建/关闭 session 等）
	locksMutex     sync.Mutex
	perServerLocks map[string]*sync.Mutex

	// 会话ID自增序列，保证唯一
	sessionSeq uint64

	// 连接健康检查取消函数
	healthMu     sync.Mutex
	healthCancel map[string]context.CancelFunc
}

// reconnectInfo 自动重连运行时信息
type reconnectInfo struct {
	enabled  bool
	attempts int
	cancel   context.CancelFunc
}

// loadPortForwardConfigs 加载持久化的端口转发配置
func (sc *SSHController) loadPortForwardConfigs() {
	data, err := os.ReadFile("config/portforwards.json")
	if err != nil {
		return
	}
	var loaded map[string][]services.PortForward
	if err := json.Unmarshal(data, &loaded); err == nil {
		sc.pfMutex.Lock()
		sc.portForwardCfgs = loaded
		sc.pfMutex.Unlock()
	}
}

// savePortForwardConfigs 持久化端口转发配置
func (sc *SSHController) savePortForwardConfigs() error {
	sc.pfMutex.Lock()
	defer sc.pfMutex.Unlock()
	data, err := json.MarshalIndent(sc.portForwardCfgs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("config/portforwards.json", data, 0644)
}

// saveAutoReconnect 持久化自动重连开关
func (sc *SSHController) saveAutoReconnect() error {
	sc.reconnectMu.Lock()
	defer sc.reconnectMu.Unlock()
	data, err := json.MarshalIndent(sc.autoReconnect, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("config/autoreconnect.json", data, 0644)
}

// loadAutoReconnect 加载自动重连开关
func (sc *SSHController) loadAutoReconnect() {
	data, err := os.ReadFile("config/autoreconnect.json")
	if err != nil {
		return
	}
	var loaded map[string]bool
	if err := json.Unmarshal(data, &loaded); err == nil {
		sc.autoReconnect = loaded
	}
}

// isAutoReconnectEnabled 是否启用自动重连
func (sc *SSHController) isAutoReconnectEnabled(serverID string) bool {
	sc.reconnectMu.Lock()
	defer sc.reconnectMu.Unlock()
	return sc.autoReconnect[serverID]
}

// NewSSHController 创建新的SSH控制器
func NewSSHController(app *application.App) *SSHController {
	// 加密密码优先从环境变量读取，避免硬编码；未设置时使用默认值（保持向后兼容）
	password := os.Getenv("GO_TERM_ENC_KEY")
	if password == "" {
		password = "androidsr"
	}

	return &SSHController{
		app:                app,
		connections:        make(map[string]*services.SSHConnection),
		sftpClients:        make(map[string]*sftp.Client),
		terminalSessions:   make(map[string]*services.TerminalSession),
		sessionServer:      make(map[string]string),
		perServerLocks:     make(map[string]*sync.Mutex),
		configFile:         "config/servers.dat", // 默认使用加密文件扩展名
		useEncryption:      true,                 // 默认启用加密
		needReencrypt:      false,                // 默认不需要重新加密
		scriptManager:      services.NewScriptManager(),
		scriptParser:       services.NewScriptParser(),
		enhancedExecutor:   services.NewEnhancedScriptExecutor(),
		encryptionPassword: password,
		opsConfigManager:   services.NewOpsConfigManager(),
		lastPaths:          services.NewLastPathStore(),
		commandHistory:     services.NewCommandHistoryStore(),
		snippetManager:     services.NewSnippetManager(),
		portForwardMgr:     services.NewPortForwardManager(),
		portForwardCfgs:    make(map[string][]services.PortForward),
		reconnectState:     make(map[string]*reconnectInfo),
		autoReconnect:      make(map[string]bool),
		healthCancel:       make(map[string]context.CancelFunc),
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

// generateSessionID 生成全局唯一的会话ID
func (sc *SSHController) generateSessionID(serverID string) string {
	seq := atomic.AddUint64(&sc.sessionSeq, 1)
	return fmt.Sprintf("sess_%s_%d", serverID, seq)
}

// ServiceStartup 实现 v3 服务生命周期接口，应用启动时初始化控制器
func (sc *SSHController) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
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

	// 加载运维配置
	if err := sc.opsConfigManager.LoadFromFile("config/opsconfig.json"); err != nil {
		fmt.Printf("警告: 无法加载运维配置: %v\n", err)
	}

	// 加载上次打开路径
	if err := sc.lastPaths.Load("config/lastpaths.json"); err != nil {
		fmt.Printf("警告: 无法加载上次路径记录: %v\n", err)
	}

	// 加载命令历史 / 片段库 / 端口转发配置 / 自动重连开关
	if err := sc.commandHistory.Load("config/command_history.json"); err != nil {
		fmt.Printf("警告: 无法加载命令历史: %v\n", err)
	}
	if err := sc.snippetManager.Load("config/snippets.json"); err != nil {
		fmt.Printf("警告: 无法加载片段库: %v\n", err)
	}
	sc.loadPortForwardConfigs()
	sc.loadAutoReconnect()

	return nil
}

// ServiceShutdown 应用关闭时保存运维配置与路径记录
func (sc *SSHController) ServiceShutdown(ctx context.Context, options application.ServiceOptions) error {
	_ = sc.opsConfigManager.SaveToFile("config/opsconfig.json")
	_ = sc.lastPaths.Save("config/lastpaths.json")
	_ = sc.commandHistory.Save()
	_ = sc.snippetManager.Save()
	_ = sc.savePortForwardConfigs()
	_ = sc.saveAutoReconnect()
	return nil
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

// ConnectToServer 连接到服务器（支持通过跳板机代理连接）
func (sc *SSHController) ConnectToServer(serverID string) (string, error) {
	// 先读取服务器配置 & 当前连接状态（短锁）
	sc.mutex.RLock()
	_, already := sc.connections[serverID]
	sc.mutex.RUnlock()

	if already {
		return "已连接到服务器", nil
	}

	// 从 serverManager 获取 server 信息
	server, err := sc.serverManager.GetServerByID(serverID)
	if err != nil {
		return "", fmt.Errorf("无法找到服务器: %v", err)
	}

	// 创建连接是在无全局锁下进行的耗时 IO
	connection := &services.SSHConnection{}
	if server.ProxyJumpServerID != "" {
		// 通过跳板机代理连接
		sc.mutex.RLock()
		proxyConn, ok := sc.connections[server.ProxyJumpServerID]
		sc.mutex.RUnlock()
		if !ok || proxyConn == nil || proxyConn.Client == nil {
			return "", fmt.Errorf("跳板机尚未连接，请先连接跳板机服务器")
		}
		if err := connection.ConnectWithProxy(server.Host, server.Port, server.Username, server.Password, server.KeyFile, proxyConn.Client); err != nil {
			return "", fmt.Errorf("通过跳板机连接失败: %v", err)
		}
	} else {
		if err := connection.Connect(server.Host, server.Port, server.Username, server.Password, server.KeyFile); err != nil {
			return "", fmt.Errorf("连接失败: %v", err)
		}
	}

	// 成功后将连接写入 map（短锁）
	sc.mutex.Lock()
	// double-check 避免竞态：可能在我们创建期间别人已创建
	if existing, ok := sc.connections[serverID]; ok && existing.Client != nil {
		// 我们的 connection 多余，先 close 掉自己
		sc.mutex.Unlock()
		connection.Close()
		return "已连接到服务器", nil
	}
	sc.connections[serverID] = connection
	sc.mutex.Unlock()

	// 启动连接健康检查
	sc.startHealthMonitor(serverID)

	// 恢复该服务器保存的端口转发
	sc.restorePortForwards(serverID)

	return "连接成功", nil
}

// ExecuteCommand 在服务器上执行命令（直接通过连接执行，不依赖终端会话，供批量脚本执行器使用）
func (sc *SSHController) ExecuteCommand(serverID, command string) (string, error) {
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
	// 停止健康检查
	sc.stopHealthMonitor(serverID)

	// 停止该服务器下的端口转发
	sc.portForwardMgr.StopAll(serverID)

	// 取消正在进行的自动重连
	sc.reconnectMu.Lock()
	if info, ok := sc.reconnectState[serverID]; ok {
		info.enabled = false
		if info.cancel != nil {
			info.cancel()
		}
		delete(sc.reconnectState, serverID)
	}
	sc.reconnectMu.Unlock()

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
func (sc *SSHController) IsTerminalSessionActive(sessionID string) bool {
	sc.mutex.RLock()
	session, exists := sc.terminalSessions[sessionID]
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

// CreateTerminalSessionWithSize 创建指定尺寸的终端会话，返回唯一 sessionID
// 一个服务器可创建多个终端会话（支持"复制会话"）。
func (sc *SSHController) CreateTerminalSessionWithSize(serverID string, width, height int) (string, error) {
	// 1. 检查连接状态
	if !sc.isConnectionHealthy(serverID) {
		return "", fmt.Errorf("服务器连接无效，请重新连接")
	}

	// 检查现有会话是否有效（多会话模式下不做单 server 去重，允许同时存在多个）
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sc.mutex.RUnlock()

	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}

	// 2. 使用 per-server lock 序列化本服务器的 create/close 操作
	serverLock := sc.getServerLock(serverID)
	serverLock.Lock()
	defer serverLock.Unlock()

	// createTerminal 是耗时 IO —— 必须在没有持有全局 sc.mutex 的情况下执行
	terminalSession, err := conn.CreateTerminalSession(width, height)
	if err != nil {
		return "", fmt.Errorf("创建终端会话失败: %v", err)
	}

	// 生成唯一 sessionID
	sessionID := sc.generateSessionID(serverID)

	// 创建成功后用短锁写回 map
	sc.mutex.Lock()
	sc.terminalSessions[sessionID] = terminalSession
	sc.sessionServer[sessionID] = serverID
	sc.mutex.Unlock()

	// 设置意外断开回调（用于检测 vi 卡死/服务器掉线等）
	terminalSession.OnUnexpectedClose(func() {
		sc.handleSessionUnexpectedClose(serverID, sessionID)
	})

	// 设置事件推送函数并启动推送协程（事件以 sessionID 为维度，避免多会话互相串扰）
	terminalSession.SetEventEmitter(sessionID, func(event string, data ...interface{}) {
		sc.app.Event.Emit(event, data...)
	})
	terminalSession.StartOutputPusher()

	return sessionID, nil
}

// handleSessionUnexpectedClose 终端会话意外断开（连接已死、远端关闭等）时的统一处理
func (sc *SSHController) handleSessionUnexpectedClose(serverID, sessionID string) {
	sc.mutex.Lock()
	delete(sc.terminalSessions, sessionID)
	delete(sc.sessionServer, sessionID)
	sc.mutex.Unlock()

	// 通知前端该会话已失效，便于弹出重连/退出提示
	if sc.app != nil {
		sc.app.Event.Emit("terminal-session-closed", map[string]interface{}{
			"sessionID": sessionID,
			"serverID":  serverID,
			"reason":    "连接已断开或远端关闭了会话",
		})
	}
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
func (sc *SSHController) ReadTerminalOutput(sessionID string) (string, error) {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[sessionID]
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
func (sc *SSHController) GetTerminalLastOutput(sessionID string) (string, error) {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[sessionID]
	sc.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("终端会话不存在")
	}

	return terminalSession.GetLastOutput(), nil
}

// ClearTerminalOutputBuffer 清空终端输出缓冲区
func (sc *SSHController) ClearTerminalOutputBuffer(sessionID string) error {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[sessionID]
	sc.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("终端会话不存在")
	}

	terminalSession.ClearOutputBuffer()
	return nil
}

// GetAutoCompleteSuggestions 获取自动补全建议
func (sc *SSHController) GetAutoCompleteSuggestions(sessionID, partialCommand string) ([]string, error) {
	sc.mutex.RLock()
	terminalSession, exists := sc.terminalSessions[sessionID]
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
	if err := conn.UploadFile(sftpClient, localPath, remotePath, nil); err != nil {
		return "", fmt.Errorf("上传文件失败: %v", err)
	}
	return "文件上传成功", nil
}

// UploadFileWithProgress 带进度回调的上传文件（任务化：进度事件携带 taskID）
func (sc *SSHController) UploadFileWithProgress(serverID, taskID, localPath, remotePath string) (string, error) {
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

	// 带进度回调的上传
	if err := conn.UploadFile(sftpClient, localPath, remotePath, func(transferred, total int64) {
		// 发送进度事件到前端（携带 taskID，便于前端聚合为任务列表）
		percent := float64(0)
		if total > 0 {
			percent = float64(transferred) / float64(total) * 100
		}
		sc.app.Event.Emit("file-upload-progress", map[string]interface{}{
			"serverID":    serverID,
			"taskID":      taskID,
			"localPath":   localPath,
			"remotePath":  remotePath,
			"transferred": transferred,
			"total":       total,
			"percent":     percent,
		})
	}); err != nil {
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
	if err := conn.DownloadFile(sftpClient, remotePath, localPath, nil); err != nil {
		return "", fmt.Errorf("下载文件失败: %v", err)
	}
	return "文件下载成功", nil
}

// DownloadFileWithProgress 带进度回调的下载文件（任务化：进度事件携带 taskID）
func (sc *SSHController) DownloadFileWithProgress(serverID, taskID, remotePath, localPath string) (string, error) {
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

	// 带进度回调的下载
	if err := conn.DownloadFile(sftpClient, remotePath, localPath, func(transferred, total int64) {
		// 发送进度事件到前端（携带 taskID，便于前端聚合为任务列表）
		percent := float64(0)
		if total > 0 {
			percent = float64(transferred) / float64(total) * 100
		}
		sc.app.Event.Emit("file-download-progress", map[string]interface{}{
			"serverID":   serverID,
			"taskID":     taskID,
			"remotePath": remotePath,
			"localPath":  localPath,
			"transferred": transferred,
			"total":      total,
			"percent":    percent,
		})
	}); err != nil {
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

// ExecuteCommandWithoutNewline 执行命令但不添加换行符（按 sessionID 定向）
func (sc *SSHController) ExecuteCommandWithoutNewline(sessionID, command string) (string, error) {
	// 优先检查是否存在终端会话（短锁）
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[sessionID]
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

// InterruptCommand 中断当前正在执行的命令（发送 Ctrl+C，按 sessionID 定向）
func (sc *SSHController) InterruptCommand(sessionID string) (string, error) {
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[sessionID]
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

// CloseTerminalSession 关闭指定的终端会话（按 sessionID 定向）
func (sc *SSHController) CloseTerminalSession(sessionID string) (string, error) {
	// 先从 sessionServer 反查 serverID，保证与创建/上传等操作用同一把 per-server 锁
	sc.mutex.RLock()
	serverID, ok := sc.sessionServer[sessionID]
	sc.mutex.RUnlock()
	if !ok {
		return "终端会话不存在", nil
	}
	// 序列化同 server 的操作
	serverLock := sc.getServerLock(serverID)
	serverLock.Lock()
	defer serverLock.Unlock()
	// 读取会话副本（短锁），然后释放锁进行关闭
	sc.mutex.RLock()
	session, hasSession := sc.terminalSessions[sessionID]
	sc.mutex.RUnlock()

	if !hasSession {
		return "终端会话不存在", nil
	}

	var errMsg string

	// 使用更严格的超时控制
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	fmt.Println("会话副本读取完成", sessionID)

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
			log.Printf("终端会话已断开连接: %v", sessionID)
		}
	case <-ctx.Done():
		errMsg = "关闭终端会话超时"
		log.Printf("关闭终端会话超时，强制终止")
		// 在超时情况下，尝试强制清理资源
	}

	// 确保清理数据结构（短锁）
	sc.mutex.Lock()
	delete(sc.terminalSessions, sessionID)
	delete(sc.sessionServer, sessionID)
	sc.mutex.Unlock()

	if errMsg != "" {
		return "", fmt.Errorf("%s", errMsg)
	}
	return "终端会话已关闭", nil
}

// ResizeTerminal 调整终端大小（按 sessionID 定向）
func (sc *SSHController) ResizeTerminal(sessionID string, width, height int) (string, error) {
	// 读取终端会话（短锁）
	sc.mutex.RLock()
	session, exists := sc.terminalSessions[sessionID]
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

// ========== 连接健康检查与断线通知 ==========

// startHealthMonitor 启动针对某服务器的后台健康检查，连接断开时通知前端
func (sc *SSHController) startHealthMonitor(serverID string) {
	sc.healthMu.Lock()
	// 若已存在监控，先取消旧监控
	if cancel, ok := sc.healthCancel[serverID]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	sc.healthCancel[serverID] = cancel
	sc.healthMu.Unlock()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sc.mutex.RLock()
				conn, exists := sc.connections[serverID]
				sc.mutex.RUnlock()
				if !exists || conn == nil || conn.Client == nil {
					// 连接已不存在，停止监控
					cancel()
					return
				}
				_, _, err := conn.Client.SendRequest("keepalive@openssh.com", true, nil)
				if err != nil {
					// 连接已失效
					sc.handleConnectionLost(serverID)
					return
				}
			}
		}
	}()
}

// stopHealthMonitor 停止健康检查
func (sc *SSHController) stopHealthMonitor(serverID string) {
	sc.healthMu.Lock()
	if cancel, ok := sc.healthCancel[serverID]; ok {
		cancel()
		delete(sc.healthCancel, serverID)
	}
	sc.healthMu.Unlock()
}

// handleConnectionLost 连接断开时的统一处理：清理资源，并按配置决定是否自动重连
func (sc *SSHController) handleConnectionLost(serverID string) {
	sc.stopHealthMonitor(serverID)

	// 清理该服务器下的所有终端会话
	sc.cleanupServerSessions(serverID)

	// 清理连接与 SFTP
	sc.mutex.Lock()
	if sftpClient, ok := sc.sftpClients[serverID]; ok {
		_ = sftpClient.Close()
		delete(sc.sftpClients, serverID)
	}
	if conn, ok := sc.connections[serverID]; ok {
		conn.Close()
		delete(sc.connections, serverID)
	}
	sc.mutex.Unlock()

	// 自动重连开启则进入重连流程（保留前端标签，避免误关）；否则通知前端断开
	if sc.isAutoReconnectEnabled(serverID) {
		sc.reconnectMu.Lock()
		sc.reconnectState[serverID] = &reconnectInfo{enabled: true}
		sc.reconnectMu.Unlock()
		if sc.app != nil {
			sc.app.Event.Emit("reconnecting", map[string]interface{}{
				"serverID": serverID,
				"attempt":  1,
				"reason":   "连接已断开，正在尝试自动重连…",
			})
		}
		go sc.attemptReconnect(serverID)
		return
	}

	sc.emitConnectionLost(serverID, "服务器连接已断开（网络中断或远端关闭）")
}

// emitConnectionLost 通知前端连接已彻底断开
func (sc *SSHController) emitConnectionLost(serverID, reason string) {
	if sc.app != nil {
		sc.app.Event.Emit("connection-lost", map[string]interface{}{
			"serverID": serverID,
			"reason":   reason,
		})
	}
}

// restorePortForwards 连接成功后恢复该服务器保存的端口转发
func (sc *SSHController) restorePortForwards(serverID string) {
	sc.pfMutex.Lock()
	cfgs := sc.portForwardCfgs[serverID]
	sc.pfMutex.Unlock()
	sc.mutex.RLock()
	conn := sc.connections[serverID]
	sc.mutex.RUnlock()
	if conn == nil || conn.Client == nil || len(cfgs) == 0 {
		return
	}
	for _, fwd := range cfgs {
		_ = sc.portForwardMgr.Start(conn.Client, fwd)
	}
}

// EnableAutoReconnect 设置某服务器是否启用自动重连；关闭时取消正在进行的重连
func (sc *SSHController) EnableAutoReconnect(serverID string, enabled bool) error {
	sc.reconnectMu.Lock()
	if enabled {
		sc.autoReconnect[serverID] = true
	} else {
		delete(sc.autoReconnect, serverID)
		if info, ok := sc.reconnectState[serverID]; ok {
			info.enabled = false
			if info.cancel != nil {
				info.cancel()
			}
		}
		delete(sc.reconnectState, serverID)
	}
	sc.reconnectMu.Unlock()
	_ = sc.saveAutoReconnect()
	return nil
}

// GetAutoReconnect 获取某服务器是否启用自动重连
func (sc *SSHController) GetAutoReconnect(serverID string) bool {
	return sc.isAutoReconnectEnabled(serverID)
}

// attemptReconnect 按指数退避自动重连，成功后恢复转发与监控
func (sc *SSHController) attemptReconnect(serverID string) {
	server, err := sc.serverManager.GetServerByID(serverID)
	if err != nil {
		sc.reconnectMu.Lock()
		delete(sc.reconnectState, serverID)
		sc.reconnectMu.Unlock()
		sc.emitConnectionLost(serverID, "服务器配置丢失，无法重连")
		return
	}
	maxAttempts := 8
	backoff := 2 * time.Second
	for i := 1; i <= maxAttempts; i++ {
		// 检查是否被取消 / 已被手动恢复
		sc.reconnectMu.Lock()
		info := sc.reconnectState[serverID]
		if info == nil || !info.enabled {
			sc.reconnectMu.Unlock()
			return
		}
		info.attempts = i
		sc.reconnectMu.Unlock()

		// 若连接已被手动恢复，直接结束
		sc.mutex.RLock()
		_, already := sc.connections[serverID]
		sc.mutex.RUnlock()
		if already {
			sc.reconnectMu.Lock()
			delete(sc.reconnectState, serverID)
			sc.reconnectMu.Unlock()
			return
		}

		if i > 1 {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 2*time.Minute {
				backoff = 2 * time.Minute
			}
		}

		// 再次检查取消
		sc.reconnectMu.Lock()
		if sc.reconnectState[serverID] == nil || !sc.reconnectState[serverID].enabled {
			sc.reconnectMu.Unlock()
			return
		}
		sc.reconnectMu.Unlock()

		if sc.app != nil {
			sc.app.Event.Emit("reconnecting", map[string]interface{}{
				"serverID": serverID,
				"attempt":  i,
				"reason":   fmt.Sprintf("正在第 %d/%d 次重连…", i, maxAttempts),
			})
		}

		connection := &services.SSHConnection{}
		var cerr error
		if server.ProxyJumpServerID != "" {
			sc.mutex.RLock()
			proxyConn := sc.connections[server.ProxyJumpServerID]
			sc.mutex.RUnlock()
			if proxyConn == nil || proxyConn.Client == nil {
				cerr = fmt.Errorf("跳板机未连接")
			} else {
				cerr = connection.ConnectWithProxy(server.Host, server.Port, server.Username, server.Password, server.KeyFile, proxyConn.Client)
			}
		} else {
			cerr = connection.Connect(server.Host, server.Port, server.Username, server.Password, server.KeyFile)
		}

		if cerr == nil {
			sc.mutex.Lock()
			if _, exists := sc.connections[serverID]; !exists {
				sc.connections[serverID] = connection
			} else {
				connection.Close()
			}
			sc.mutex.Unlock()
			sc.startHealthMonitor(serverID)
			sc.restorePortForwards(serverID)
			sc.reconnectMu.Lock()
			delete(sc.reconnectState, serverID)
			sc.reconnectMu.Unlock()
			if sc.app != nil {
				sc.app.Event.Emit("reconnected", map[string]interface{}{"serverID": serverID})
			}
			return
		}
	}
	// 放弃重连
	sc.reconnectMu.Lock()
	delete(sc.reconnectState, serverID)
	sc.reconnectMu.Unlock()
	sc.emitConnectionLost(serverID, "多次尝试重连失败，请手动重连")
}

// cleanupServerSessions 清理指定服务器下的全部终端会话（不发事件，由调用方统一通知）
func (sc *SSHController) cleanupServerSessions(serverID string) {
	sc.mutex.Lock()
	var toClose []*services.TerminalSession
	var toDelete []string
	for sid, srv := range sc.sessionServer {
		if srv == serverID {
			if sess, ok := sc.terminalSessions[sid]; ok {
				toClose = append(toClose, sess)
			}
			toDelete = append(toDelete, sid)
		}
	}
	for _, sid := range toDelete {
		delete(sc.terminalSessions, sid)
		delete(sc.sessionServer, sid)
	}
	sc.mutex.Unlock()

	for _, sess := range toClose {
		_ = sess.Close()
	}
}

// ========== 上次打开路径 ==========

// GetLastPath 获取服务器上次打开的目录
func (sc *SSHController) GetLastPath(serverID string) string {
	return sc.lastPaths.Get(serverID)
}

// SetLastPath 记录服务器上次打开的目录并落盘
func (sc *SSHController) SetLastPath(serverID, path string) string {
	sc.lastPaths.Set(serverID, path)
	if err := sc.lastPaths.Save("config/lastpaths.json"); err != nil {
		fmt.Printf("警告: 保存上次路径失败: %v\n", err)
	}
	return path
}

// ========== 运维配置模块 ==========

// GetOpsConfigs 获取所有运维配置
func (sc *SSHController) GetOpsConfigs() []models.OpsConfig {
	return sc.opsConfigManager.GetConfigs()
}

// GetOpsConfigsByServer 获取指定服务器的运维配置
func (sc *SSHController) GetOpsConfigsByServer(serverID string) []models.OpsConfig {
	return sc.opsConfigManager.GetConfigsByServer(serverID)
}

// AddOpsConfig 新增运维配置
func (sc *SSHController) AddOpsConfig(cfg models.OpsConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("请填写配置名称")
	}
	if err := sc.opsConfigManager.AddConfig(cfg); err != nil {
		return err
	}
	return sc.opsConfigManager.SaveToFile("config/opsconfig.json")
}

// UpdateOpsConfig 更新运维配置
func (sc *SSHController) UpdateOpsConfig(cfg models.OpsConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("请填写配置名称")
	}
	if err := sc.opsConfigManager.UpdateConfig(cfg); err != nil {
		return err
	}
	return sc.opsConfigManager.SaveToFile("config/opsconfig.json")
}

// DeleteOpsConfig 删除运维配置
func (sc *SSHController) DeleteOpsConfig(id string) error {
	if err := sc.opsConfigManager.DeleteConfig(id); err != nil {
		return err
	}
	return sc.opsConfigManager.SaveToFile("config/opsconfig.json")
}

// ========== 命令历史 ==========

// GetCommandHistory 获取命令历史
func (sc *SSHController) GetCommandHistory(serverID string) []string {
	return sc.commandHistory.Get(serverID)
}

// AddCommandHistory 追加命令历史
func (sc *SSHController) AddCommandHistory(serverID, command string) {
	sc.commandHistory.Add(serverID, command)
}

// ClearCommandHistory 清空命令历史
func (sc *SSHController) ClearCommandHistory(serverID string) {
	sc.commandHistory.Clear(serverID)
}

// ========== 命令片段库 ==========

// GetSnippets 获取全部片段
func (sc *SSHController) GetSnippets() []models.Snippet {
	return sc.snippetManager.GetAll()
}

// AddSnippet 新增片段
func (sc *SSHController) AddSnippet(s models.Snippet) error {
	if s.Name == "" {
		return fmt.Errorf("请填写片段名称")
	}
	return sc.snippetManager.Add(s)
}

// UpdateSnippet 更新片段
func (sc *SSHController) UpdateSnippet(s models.Snippet) error {
	if s.Name == "" {
		return fmt.Errorf("请填写片段名称")
	}
	return sc.snippetManager.Update(s)
}

// DeleteSnippet 删除片段
func (sc *SSHController) DeleteSnippet(id string) error {
	return sc.snippetManager.Delete(id)
}

// ========== 端口转发 ==========

// AddPortForward 新增并启动端口转发
func (sc *SSHController) AddPortForward(serverID string, fwd services.PortForward) (string, error) {
	sc.mutex.RLock()
	conn, exists := sc.connections[serverID]
	sc.mutex.RUnlock()
	if !exists || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}
	if fwd.ID == "" {
		fwd.ID = fmt.Sprintf("pf_%d", time.Now().UnixNano())
	}
	fwd.ServerID = serverID
	fwd.Status = "active"
	if err := sc.portForwardMgr.Start(conn.Client, fwd); err != nil {
		return "", err
	}
	sc.pfMutex.Lock()
	sc.portForwardCfgs[serverID] = append(sc.portForwardCfgs[serverID], fwd)
	sc.pfMutex.Unlock()
	_ = sc.savePortForwardConfigs()
	return fwd.ID, nil
}

// RemovePortForward 移除端口转发
func (sc *SSHController) RemovePortForward(id string) error {
	sc.pfMutex.Lock()
	var foundServer string
	idx := -1
	for sid, list := range sc.portForwardCfgs {
		for i, f := range list {
			if f.ID == id {
				foundServer = sid
				idx = i
				break
			}
		}
		if idx >= 0 {
			break
		}
	}
	if idx >= 0 {
		list := sc.portForwardCfgs[foundServer]
		sc.portForwardCfgs[foundServer] = append(list[:idx], list[idx+1:]...)
	}
	sc.pfMutex.Unlock()

	if err := sc.portForwardMgr.Stop(id); err != nil {
		return err
	}
	_ = sc.savePortForwardConfigs()
	return nil
}

// ListPortForwards 列出端口转发
func (sc *SSHController) ListPortForwards(serverID string) []services.PortForward {
	return sc.portForwardMgr.List(serverID)
}

// ========== 快速连接（ssh://user@host:port） ==========

// ConnectByURI 解析 ssh://user[:pass]@host[:port] 或 user@host:port 一键直连（临时入库）
func (sc *SSHController) ConnectByURI(uri string) (string, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return "", fmt.Errorf("请输入连接地址")
	}
	uri = strings.TrimPrefix(uri, "ssh://")
	re := regexp.MustCompile(`^(?:([^:@]+)(?::([^@]*))?@)?([^:/]+)(?::(\d+))?$`)
	m := re.FindStringSubmatch(uri)
	if m == nil {
		return "", fmt.Errorf("无法解析连接地址: %s", uri)
	}
	user := m[1]
	password := m[2]
	host := m[3]
	portStr := m[4]
	port := 22
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			port = p
		}
	}
	if user == "" {
		user = "root"
	}

	groupID, err := sc.ensureQuickConnectGroup()
	if err != nil {
		return "", err
	}
	serverID := fmt.Sprintf("qc_%d", time.Now().UnixNano())
	server := models.Server{
		ID:       serverID,
		Name:     fmt.Sprintf("%s@%s:%d", user, host, port),
		Host:     host,
		Port:     port,
		Username: user,
		Password: password,
		GroupID:  groupID,
	}
	if err := sc.serverManager.AddServer(groupID, server); err != nil {
		return "", fmt.Errorf("保存快速连接失败: %v", err)
	}
	_ = sc.saveConfig()

	if _, err := sc.ConnectToServer(serverID); err != nil {
		return "", err
	}
	return serverID, nil
}

// ensureQuickConnectGroup 确保“快速连接”分组存在并返回其 ID
func (sc *SSHController) ensureQuickConnectGroup() (string, error) {
	const groupName = "快速连接"
	for _, g := range sc.serverManager.GetGroups() {
		if g.Name == groupName {
			return g.ID, nil
		}
	}
	group := models.ServerGroup{ID: "group_quickconnect", Name: groupName, Servers: []models.Server{}}
	sc.serverManager.AddGroup(group)
	if err := sc.saveConfig(); err != nil {
		return "", err
	}
	_ = sc.saveConfig()
	return group.ID, nil
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

// ExecuteBatchScript 执行批量脚本（后端批量模式，结果通过返回值一次性返回）
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

	// 通知前端：脚本任务已创建（用于实时任务列表）
	if sc.app != nil {
		sc.app.Event.Emit("script-task-created", map[string]interface{}{
			"scriptID":   scriptID,
			"scriptName": script.Name,
			"serverIDs":  script.ServerIDs,
			"serverNames": func() []string {
				names := make([]string, 0, len(script.ServerIDs))
				for _, sid := range script.ServerIDs {
					names = append(names, serverMap[sid])
				}
				return names
			}(),
		})
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

			// 通知前端：该服务器执行结果已更新（实时日志）
			if sc.app != nil {
				sc.app.Event.Emit("script-task-update", map[string]interface{}{
					"scriptID":       scriptID,
					"serverID":       sid,
					"serverName":     serverMap[sid],
					"status":         execution.Status,
					"commandOutputs": execution.CommandOutputs,
					"error":          execution.Error,
				})
			}
		}(serverID)
	}

	wg.Wait()
	return results, nil
}

// SendScriptToTerminal 逐行发送脚本命令到指定终端会话（用于命令模式）
func (sc *SSHController) SendScriptToTerminal(scriptID string, sessionID string) error {
	// 获取会话对应的服务器
	sc.mutex.RLock()
	serverID, ok := sc.sessionServer[sessionID]
	sc.mutex.RUnlock()
	if !ok {
		return fmt.Errorf("终端会话不存在或已失效")
	}

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
	sc.mutex.RLock()
	_, sessionExists := sc.terminalSessions[sessionID]
	sc.mutex.RUnlock()
	if !sessionExists {
		return fmt.Errorf("终端会话不存在或已失效")
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
			}
			// 添加一个小延迟
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// 处理本地命令（在本地执行，不发送到服务器）
		if parsedCmd.CommandType == "local" {
			// 在本地执行命令
			output, err := sc.enhancedExecutor.HandleLocalCommand(parsedCmd.Command)
			if err != nil {
				fmt.Printf("本地命令执行失败: %v\n", err)
				// 即使有错误也要显示命令
				output = fmt.Sprintf("执行错误: %v", err)
			}
			// 在前端弹出窗口显示本地命令的输出
			// 始终发送事件，即使没有输出也显示命令
			displayOutput := output
			if displayOutput == "" {
				displayOutput = "(无输出)"
			}
			sc.app.Event.Emit("local-command-output", map[string]interface{}{
				"command": "!" + parsedCmd.Command,
				"output":  displayOutput,
			})
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// 处理shell类型的命令，发送到终端
		if parsedCmd.CommandType == "shell" {
			// 发送命令到终端（带换行符，让命令执行）
			_, err = sc.ExecuteCommandWithoutNewline(sessionID, parsedCmd.Command+"\n")
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

// HandleFileUploadRequest 处理文件上传请求（供终端内嵌上传使用）
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

// HandleFileDownloadRequest 处理文件下载请求（供终端内嵌下载使用）
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
