package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"go-term/models"
)

// OpsConfigManager 运维配置管理器：集中存放用户自填的运维配置笔记
// （名称 + 关联项目 + 自由文本），避免把运维知识散落到各处。
type OpsConfigManager struct {
	configs []models.OpsConfig
	mu      sync.RWMutex
}

// NewOpsConfigManager 创建运维配置管理器
func NewOpsConfigManager() *OpsConfigManager {
	return &OpsConfigManager{
		configs: make([]models.OpsConfig, 0),
	}
}

// LoadFromFile 从文件加载运维配置（明文 JSON）
func (m *OpsConfigManager) LoadFromFile(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// 文件不存在，初始化为空配置
		m.configs = make([]models.OpsConfig, 0)
		return nil
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("无法读取运维配置文件: %v", err)
	}
	if len(data) == 0 {
		m.configs = make([]models.OpsConfig, 0)
		return nil
	}

	var loaded []models.OpsConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		// 兼容旧版本：旧字段 notes 映射到新的 content，避免已有数据丢失
		type legacyOpsConfig struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			ServerID     string `json:"serverId"`
			DeployMethod string `json:"deployMethod"`
			AppDir       string `json:"appDir"`
			StartCmd     string `json:"startCmd"`
			EnvVars      string `json:"envVars"`
			RepoURL      string `json:"repoUrl"`
			LogsDir      string `json:"logsDir"`
			Notes        string `json:"notes"`
			Content      string `json:"content"`
			UpdatedAt    string `json:"updatedAt"`
		}
		var legacy []legacyOpsConfig
		if jerr := json.Unmarshal(data, &legacy); jerr != nil {
			return fmt.Errorf("无法解析运维配置文件: %v", err)
		}
		loaded = make([]models.OpsConfig, 0, len(legacy))
		for _, c := range legacy {
			content := c.Content
			if content == "" {
				content = c.Notes // 旧数据的备注迁移为内容
			}
			loaded = append(loaded, models.OpsConfig{
				ID:        c.ID,
				Name:      c.Name,
				ServerID:  c.ServerID,
				Content:   content,
				UpdatedAt: c.UpdatedAt,
			})
		}
	}
	m.configs = loaded
	return nil
}

// SaveToFile 保存运维配置（明文 JSON）
func (m *OpsConfigManager) SaveToFile(filename string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := json.MarshalIndent(m.configs, "", "  ")
	if err != nil {
		return fmt.Errorf("无法序列化运维配置: %v", err)
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("无法写入运维配置文件: %v", err)
	}
	return nil
}

// GetConfigs 获取所有运维配置
func (m *OpsConfigManager) GetConfigs() []models.OpsConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]models.OpsConfig, len(m.configs))
	copy(out, m.configs)
	// 按更新时间倒序，最新修改的靠前
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

// GetConfigsByServer 获取指定服务器的运维配置
func (m *OpsConfigManager) GetConfigsByServer(serverID string) []models.OpsConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []models.OpsConfig
	for _, c := range m.configs {
		if c.ServerID == serverID {
			out = append(out, c)
		}
	}
	return out
}

// GetConfigByID 按ID获取运维配置
func (m *OpsConfigManager) GetConfigByID(id string) (*models.OpsConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.configs {
		if m.configs[i].ID == id {
			return &m.configs[i], nil
		}
	}
	return nil, fmt.Errorf("未找到ID为 %s 的运维配置", id)
}

// AddConfig 新增运维配置
func (m *OpsConfigManager) AddConfig(cfg models.OpsConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg.ID = fmt.Sprintf("ops_%d", time.Now().UnixNano())
	cfg.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
	m.configs = append(m.configs, cfg)
	return nil
}

// UpdateConfig 更新运维配置
func (m *OpsConfigManager) UpdateConfig(cfg models.OpsConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.configs {
		if m.configs[i].ID == cfg.ID {
			cfg.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
			m.configs[i] = cfg
			return nil
		}
	}
	return fmt.Errorf("未找到ID为 %s 的运维配置", cfg.ID)
}

// DeleteConfig 删除运维配置
func (m *OpsConfigManager) DeleteConfig(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.configs {
		if m.configs[i].ID == id {
			m.configs = append(m.configs[:i], m.configs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("未找到ID为 %s 的运维配置", id)
}
