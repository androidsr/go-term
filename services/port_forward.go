package services

import (
	"fmt"
	"io"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

// PortForward 端口转发配置。
// Type:
//   - "local"  : 本地监听 LocalAddr，流量转发到远端 RemoteHost:RemotePort
//   - "remote" : 远端监听 RemoteAddr，流量转发到本地 LocalAddr（反向隧道）
type PortForward struct {
	ID         string `json:"id"`
	ServerID   string `json:"serverId"`
	Type       string `json:"type"`
	LocalAddr  string `json:"localAddr"`  // 本地地址，如 127.0.0.1:8080
	RemoteHost string `json:"remoteHost"` // 远端目标主机（local 用）
	RemotePort string `json:"remotePort"` // 远端目标端口（local 用）
	RemoteAddr string `json:"remoteAddr"` // 远端监听地址（remote 用）
	Status     string `json:"status"`     // active / stopped
}

type portForwardEntry struct {
	config    PortForward
	sshClient *ssh.Client
	listener  net.Listener
	stop      chan struct{}
}

// PortForwardManager 管理活跃的端口转发。
type PortForwardManager struct {
	mu     sync.RWMutex
	active map[string]*portForwardEntry
}

// NewPortForwardManager 创建转发管理器
func NewPortForwardManager() *PortForwardManager {
	return &PortForwardManager{active: make(map[string]*portForwardEntry)}
}

// Start 启动一个转发（需要已建立的 ssh.Client）
func (m *PortForwardManager) Start(client *ssh.Client, fwd PortForward) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.active[fwd.ID]; ok {
		return fmt.Errorf("该转发已存在")
	}
	entry := &portForwardEntry{
		config:    fwd,
		sshClient: client,
		stop:      make(chan struct{}),
	}
	if fwd.Type == "local" {
		ln, err := net.Listen("tcp", fwd.LocalAddr)
		if err != nil {
			return fmt.Errorf("本地端口监听失败: %v", err)
		}
		entry.listener = ln
		go m.serveLocal(entry)
	} else if fwd.Type == "remote" {
		ln, err := client.Listen("tcp", fwd.RemoteAddr)
		if err != nil {
			return fmt.Errorf("远端端口监听失败: %v", err)
		}
		entry.listener = ln
		go m.serveRemote(entry)
	} else {
		return fmt.Errorf("不支持的转发类型: %s", fwd.Type)
	}
	entry.config.Status = "active"
	m.active[fwd.ID] = entry
	return nil
}

func (m *PortForwardManager) serveLocal(entry *portForwardEntry) {
	remoteAddr := net.JoinHostPort(entry.config.RemoteHost, entry.config.RemotePort)
	for {
		localConn, err := entry.listener.Accept()
		if err != nil {
			select {
			case <-entry.stop:
			default:
			}
			return
		}
		go func(lc net.Conn) {
			defer lc.Close()
			remoteConn, err := entry.sshClient.Dial("tcp", remoteAddr)
			if err != nil {
				return
			}
			defer remoteConn.Close()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(remoteConn, lc); done <- struct{}{} }()
			go func() { _, _ = io.Copy(lc, remoteConn); done <- struct{}{} }()
			<-done
		}(localConn)
	}
}

func (m *PortForwardManager) serveRemote(entry *portForwardEntry) {
	for {
		remoteConn, err := entry.listener.Accept()
		if err != nil {
			select {
			case <-entry.stop:
			default:
			}
			return
		}
		go func(rc net.Conn) {
			defer rc.Close()
			localConn, err := net.Dial("tcp", entry.config.LocalAddr)
			if err != nil {
				return
			}
			defer localConn.Close()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(localConn, rc); done <- struct{}{} }()
			go func() { _, _ = io.Copy(rc, localConn); done <- struct{}{} }()
			<-done
		}(remoteConn)
	}
}

// Stop 停止指定转发
func (m *PortForwardManager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.active[id]
	if !ok {
		return fmt.Errorf("转发不存在")
	}
	close(entry.stop)
	if entry.listener != nil {
		_ = entry.listener.Close()
	}
	delete(m.active, id)
	return nil
}

// List 列出某服务器的转发（含实时状态）
func (m *PortForwardManager) List(serverID string) []PortForward {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []PortForward
	for _, e := range m.active {
		if e.config.ServerID == serverID {
			out = append(out, e.config)
		}
	}
	return out
}

// StopAll 停止某服务器的全部转发
func (m *PortForwardManager) StopAll(serverID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.active {
		if e.config.ServerID == serverID {
			close(e.stop)
			if e.listener != nil {
				_ = e.listener.Close()
			}
			delete(m.active, id)
		}
	}
}
