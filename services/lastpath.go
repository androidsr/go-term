package services

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

// LastPathStore 记录每个服务器上次在文件管理器中打开的目录，
// 避免每次都要重新选择路径。
type LastPathStore struct {
	paths map[string]string
	mu    sync.RWMutex
}

// NewLastPathStore 创建路径记录存储
func NewLastPathStore() *LastPathStore {
	return &LastPathStore{
		paths: make(map[string]string),
	}
}

// Load 从文件加载
func (s *LastPathStore) Load(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		s.paths = make(map[string]string)
		return nil
	}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		s.paths = make(map[string]string)
		return nil
	}
	return json.Unmarshal(data, &s.paths)
}

// Save 保存到文件
func (s *LastPathStore) Save(filename string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, err := json.MarshalIndent(s.paths, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(filename, data, 0644)
}

// Get 获取服务器上次打开的目录，未记录时返回默认根目录 "/"
func (s *LastPathStore) Get(serverID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.paths[serverID]; ok && p != "" {
		return p
	}
	return "/"
}

// Set 记录服务器上次打开的目录
func (s *LastPathStore) Set(serverID, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths[serverID] = path
}
