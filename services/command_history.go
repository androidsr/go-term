package services

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// CommandHistoryStore 按服务器ID持久化命令历史，供终端 ↑ 回看 / 片段库以外复用。
type CommandHistoryStore struct {
	mu        sync.RWMutex
	histories map[string][]string // serverID -> 命令列表（旧->新）
	filePath  string
}

const maxHistoryPerServer = 200

// NewCommandHistoryStore 创建命令历史存储器
func NewCommandHistoryStore() *CommandHistoryStore {
	return &CommandHistoryStore{
		histories: make(map[string][]string),
	}
}

// Load 从文件加载命令历史
func (s *CommandHistoryStore) Load(filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filePath = filePath
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var loaded map[string][]string
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("无法解析命令历史: %v", err)
	}
	s.histories = loaded
	return nil
}

// Save 落盘命令历史
func (s *CommandHistoryStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.filePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(s.histories, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// Get 获取某服务器的命令历史（旧->新）
func (s *CommandHistoryStore) Get(serverID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0)
	if h, ok := s.histories[serverID]; ok {
		out = append(out, h...)
	}
	return out
}

// Add 追加一条命令（去重、限制长度、自动落盘）
func (s *CommandHistoryStore) Add(serverID, command string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.histories[serverID]
	// 与最近一条相同则跳过
	if len(list) > 0 && list[len(list)-1] == command {
		return
	}
	// 移除更早出现的相同命令，保持最新唯一
	for i := len(list) - 1; i >= 0; i-- {
		if list[i] == command {
			list = append(list[:i], list[i+1:]...)
		}
	}
	list = append(list, command)
	if len(list) > maxHistoryPerServer {
		list = list[len(list)-maxHistoryPerServer:]
	}
	s.histories[serverID] = list
	_ = s.Save()
}

// Clear 清空某服务器的命令历史
func (s *CommandHistoryStore) Clear(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.histories, serverID)
	_ = s.Save()
}
