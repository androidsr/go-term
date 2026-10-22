package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AIConfig OpenAI 兼容大模型接口配置
type AIConfig struct {
	Enabled      bool    `json:"enabled"`      // 是否启用 AI 能力
	BaseURL      string  `json:"baseURL"`      // OpenAI 兼容接口地址，如 https://api.openai.com/v1
	APIKey       string  `json:"apiKey"`       // API Key
	Model        string  `json:"model"`        // 模型名称，如 gpt-4o-mini
	Temperature  float64 `json:"temperature"`  // 温度参数
	MaxTokens    int     `json:"maxTokens"`    // 单次回复最大 token
	MaxSteps     int     `json:"maxSteps"`     // Agent 单任务最大执行轮次
	MemoryRounds int     `json:"memoryRounds"` // 跨任务记忆轮数上限（0 表示不记忆）
	SystemHint   string  `json:"systemHint"`   // 附加系统提示词（用户自定义补充）
	UseProxy     bool    `json:"useProxy"`     // 是否通过系统代理访问接口（访问内网网关时应关闭）
}

// DefaultAIConfig 返回默认 AI 配置
func DefaultAIConfig() *AIConfig {
	return &AIConfig{
		Enabled:      false,
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "",
		Model:        "gpt-4o-mini",
		Temperature:  0.3,
		MaxTokens:    2048,
		MaxSteps:     25,
		MemoryRounds: 10,
		SystemHint:   "",
		UseProxy:     false, // 默认直连：内网网关走系统代理会失败
	}
}

// AIConfigManager AI 配置管理器（负责持久化）
type AIConfigManager struct {
	mu   sync.Mutex
	path string
	cfg  *AIConfig
}

// NewAIConfigManager 创建 AI 配置管理器
func NewAIConfigManager() *AIConfigManager {
	return &AIConfigManager{
		cfg: DefaultAIConfig(),
	}
}

// Load 从文件加载配置
func (m *AIConfigManager) Load(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.path = path
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	cfg := DefaultAIConfig()
	if err := json.Unmarshal(data, cfg); err == nil {
		m.cfg = cfg
	}
}

// Save 保存配置到文件（自动创建父目录）
func (m *AIConfigManager) Save(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if path != "" {
		m.path = path
	}
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	// 确保父目录存在（相对路径 config/ 在 exe 工作目录下可能不存在）
	if dir := filepath.Dir(m.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建配置目录失败: %v", err)
		}
	}
	return os.WriteFile(m.path, data, 0644)
}

// Get 获取当前配置
func (m *AIConfigManager) Get() AIConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return *m.cfg
}

// Set 更新配置
func (m *AIConfigManager) Set(cfg AIConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = &cfg
}
