package services

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"go-term/models"
)

// SnippetManager 片段库管理器（明文 JSON 持久化）
type SnippetManager struct {
	mu       sync.RWMutex
	snippets []models.Snippet
	filePath string
}

// NewSnippetManager 创建片段管理器
func NewSnippetManager() *SnippetManager {
	return &SnippetManager{snippets: make([]models.Snippet, 0)}
}

// Load 加载片段
func (m *SnippetManager) Load(filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.filePath = filePath
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
	var loaded []models.Snippet
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("无法解析片段库: %v", err)
	}
	m.snippets = loaded
	return nil
}

// Save 落盘
func (m *SnippetManager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.filePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(m.snippets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

// GetAll 获取全部片段
func (m *SnippetManager) GetAll() []models.Snippet {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.Snippet, len(m.snippets))
	copy(out, m.snippets)
	return out
}

// Add 新增片段
func (m *SnippetManager) Add(s models.Snippet) error {
	if s.Name == "" {
		return fmt.Errorf("请填写片段名称")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Format("2006-01-02 15:04:05")
	s.ID = fmt.Sprintf("snip_%d", time.Now().UnixNano())
	s.CreatedAt = now
	s.UpdatedAt = now
	m.snippets = append(m.snippets, s)
	return m.Save()
}

// Update 更新片段
func (m *SnippetManager) Update(s models.Snippet) error {
	if s.Name == "" {
		return fmt.Errorf("请填写片段名称")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, item := range m.snippets {
		if item.ID == s.ID {
			s.CreatedAt = item.CreatedAt
			s.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
			m.snippets[i] = s
			return m.Save()
		}
	}
	return fmt.Errorf("未找到该片段")
}

// Delete 删除片段
func (m *SnippetManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, item := range m.snippets {
		if item.ID == id {
			m.snippets = append(m.snippets[:i], m.snippets[i+1:]...)
			return m.Save()
		}
	}
	return fmt.Errorf("未找到该片段")
}
