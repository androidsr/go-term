package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go-term/models"
)

// ScriptManager 脚本管理器
type ScriptManager struct {
	scripts []models.BatchScript
	mutex   sync.RWMutex
	configFile string
}

// NewScriptManager 创建新的脚本管理器
func NewScriptManager() *ScriptManager {
	return &ScriptManager{
		scripts:    make([]models.BatchScript, 0),
		configFile: "config/scripts.json",
	}
}

// LoadFromFile 从文件加载脚本配置
func (sm *ScriptManager) LoadFromFile(filename string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.configFile = filename
	
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// 文件不存在，创建空的配置
		return sm.saveToFile()
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("读取脚本配置文件失败: %v", err)
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &sm.scripts); err != nil {
			return fmt.Errorf("解析脚本配置失败: %v", err)
		}
	}

	return nil
}

// saveToFile 保存脚本配置到文件
func (sm *ScriptManager) saveToFile() error {
	// 确保目录存在
	dir := filepath.Dir(sm.configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	data, err := json.MarshalIndent(sm.scripts, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化脚本配置失败: %v", err)
	}

	if err := os.WriteFile(sm.configFile, data, 0644); err != nil {
		return fmt.Errorf("写入脚本配置文件失败: %v", err)
	}

	return nil
}

// GetScripts 获取所有脚本
func (sm *ScriptManager) GetScripts() []models.BatchScript {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// 返回副本避免外部修改
	scripts := make([]models.BatchScript, len(sm.scripts))
	copy(scripts, sm.scripts)
	return scripts
}

// GetScriptByID 根据ID获取脚本
func (sm *ScriptManager) GetScriptByID(id string) (*models.BatchScript, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	for _, script := range sm.scripts {
		if script.ID == id {
			return &script, nil
		}
	}
	return nil, fmt.Errorf("未找到脚本: %s", id)
}

// AddScript 添加脚本
func (sm *ScriptManager) AddScript(script models.BatchScript) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 检查ID是否重复
	for _, s := range sm.scripts {
		if s.ID == script.ID {
			return fmt.Errorf("脚本ID已存在: %s", script.ID)
		}
	}

	// 设置时间
	now := time.Now().Format("2006-01-02 15:04:05")
	script.CreatedAt = now
	script.UpdatedAt = now

	sm.scripts = append(sm.scripts, script)
	return sm.saveToFile()
}

// UpdateScript 更新脚本
func (sm *ScriptManager) UpdateScript(script models.BatchScript) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for i, s := range sm.scripts {
		if s.ID == script.ID {
			// 保持创建时间
			script.CreatedAt = s.CreatedAt
			script.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
			sm.scripts[i] = script
			return sm.saveToFile()
		}
	}
	return fmt.Errorf("未找到脚本: %s", script.ID)
}

// DeleteScript 删除脚本
func (sm *ScriptManager) DeleteScript(id string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	for i, script := range sm.scripts {
		if script.ID == id {
			sm.scripts = append(sm.scripts[:i], sm.scripts[i+1:]...)
			return sm.saveToFile()
		}
	}
	return fmt.Errorf("未找到脚本: %s", id)
}