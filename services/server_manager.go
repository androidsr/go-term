package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"go-term/models"
)

// ServerManager 服务器管理器
type ServerManager struct {
	Groups []models.ServerGroup `json:"groups"`
}

// NewServerManager 创建新的服务器管理器
func NewServerManager() *ServerManager {
	return &ServerManager{
		Groups: make([]models.ServerGroup, 0),
	}
}

// LoadFromFile 从文件加载服务器配置
func (sm *ServerManager) LoadFromFile(filename string) error {
	// 如果文件不存在，创建默认配置
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		sm.createDefaultConfig()
		return sm.SaveToFile(filename)
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("无法读取配置文件: %v", err)
	}

	err = json.Unmarshal(data, sm)
	if err != nil {
		return fmt.Errorf("无法解析配置文件: %v", err)
	}

	return nil
}

// SaveToFile 保存服务器配置到文件（明文格式，用于向后兼容）
func (sm *ServerManager) SaveToFile(filename string) error {
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return fmt.Errorf("无法序列化配置: %v", err)
	}

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("无法写入配置文件: %v", err)
	}

	return nil
}

// SaveToEncryptedFile 保存服务器配置到加密文件
func (sm *ServerManager) SaveToEncryptedFile(filename string, password string) error {
	// 创建加密配置管理器
	ecm := NewEncryptedConfigManager(password)

	// 保存加密配置
	err := ecm.SaveEncryptedServerManager(sm, filename)
	if err != nil {
		return fmt.Errorf("无法保存加密配置文件: %v", err)
	}

	return nil
}

// LoadFromEncryptedFile 从加密文件加载服务器配置
func (sm *ServerManager) LoadFromEncryptedFile(filename string, password string) error {
	// 如果文件不存在，创建默认配置
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		sm.createDefaultConfig()
		return sm.SaveToEncryptedFile(filename, password)
	}

	// 创建加密配置管理器
	ecm := NewEncryptedConfigManager(password)

	// 加载加密配置
	loadedSM, err := ecm.LoadEncryptedServerManager(filename)
	if err != nil {
		return fmt.Errorf("无法加载加密配置文件: %v", err)
	}

	// 更新当前实例
	sm.Groups = loadedSM.Groups

	return nil
}

// LoadFromFileWithFallback 从文件加载配置，支持明文和加密格式的自动识别
func (sm *ServerManager) LoadFromFileWithFallback(filename string, password string) (bool, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		// 文件不存在，创建默认配置
		sm.createDefaultConfig()
		return true, nil // 需要保存为加密格式
	}

	// 读取文件内容以判断格式
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return false, fmt.Errorf("无法读取配置文件: %v", err)
	}

	// 尝试以JSON格式解析（明文格式）
	var tempSM ServerManager
	if json.Unmarshal(data, &tempSM) == nil {
		// 成功解析为JSON，说明是明文格式
		*sm = tempSM
		return true, nil // 需要保存为加密格式
	}

	// 尝试以加密格式解析
	ecm := NewEncryptedConfigManager(password)
	loadedSM, err := ecm.LoadEncryptedServerManager(filename)
	if err != nil {
		return false, fmt.Errorf("无法解析配置文件（既不是有效的JSON也不是有效的加密格式）: %v", err)
	}

	// 成功解析为加密格式
	sm.Groups = loadedSM.Groups
	return false, nil // 不需要重新保存，已经是加密格式
}

// createDefaultConfig 创建默认配置
func (sm *ServerManager) createDefaultConfig() {
	defaultGroup := models.ServerGroup{
		ID:   "group1",
		Name: "默认分组",
		Servers: []models.Server{
			{
				ID:       "server1",
				Name:     "示例服务器",
				Host:     "192.168.1.100",
				Port:     22,
				Username: "root",
				Password: "",
				KeyFile:  "",
				GroupID:  "group1",
			},
		},
	}
	sm.Groups = append(sm.Groups, defaultGroup)
}

// GetGroups 获取所有分组
func (sm *ServerManager) GetGroups() []models.ServerGroup {
	return sm.Groups
}

// AddGroup 添加分组
func (sm *ServerManager) AddGroup(group models.ServerGroup) {
	sm.Groups = append(sm.Groups, group)
}

// UpdateGroup 更新分组
func (sm *ServerManager) UpdateGroup(updatedGroup models.ServerGroup) error {
	for i, group := range sm.Groups {
		if group.ID == updatedGroup.ID {
			sm.Groups[i] = updatedGroup
			return nil
		}
	}
	return fmt.Errorf("未找到ID为 %s 的分组", updatedGroup.ID)
}

// DeleteGroup 删除分组
func (sm *ServerManager) DeleteGroup(groupID string) error {
	for i, group := range sm.Groups {
		if group.ID == groupID {
			sm.Groups = append(sm.Groups[:i], sm.Groups[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("未找到ID为 %s 的分组", groupID)
}

// AddServer 添加服务器到指定分组
func (sm *ServerManager) AddServer(groupID string, server models.Server) error {
	for i, group := range sm.Groups {
		if group.ID == groupID {
			server.GroupID = groupID
			sm.Groups[i].Servers = append(sm.Groups[i].Servers, server)
			return nil
		}
	}
	return fmt.Errorf("未找到ID为 %s 的分组", groupID)
}

// UpdateServer 更新服务器信息
func (sm *ServerManager) UpdateServer(groupID string, updatedServer models.Server) error {
	for i, group := range sm.Groups {
		if group.ID == groupID {
			for j, server := range group.Servers {
				if server.ID == updatedServer.ID {
					updatedServer.GroupID = groupID
					sm.Groups[i].Servers[j] = updatedServer
					return nil
				}
			}
			return fmt.Errorf("未找到ID为 %s 的服务器", updatedServer.ID)
		}
	}
	return fmt.Errorf("未找到ID为 %s 的分组", groupID)
}

// DeleteServer 从指定分组删除服务器
func (sm *ServerManager) DeleteServer(groupID, serverID string) error {
	for i, group := range sm.Groups {
		if group.ID == groupID {
			for j, server := range group.Servers {
				if server.ID == serverID {
					sm.Groups[i].Servers = append(sm.Groups[i].Servers[:j], sm.Groups[i].Servers[j+1:]...)
					return nil
				}
			}
			return fmt.Errorf("未找到ID为 %s 的服务器", serverID)
		}
	}
	return fmt.Errorf("未找到ID为 %s 的分组", groupID)
}

// GetServerByID 根据ID获取服务器信息
func (sm *ServerManager) GetServerByID(serverID string) (*models.Server, error) {
	for _, group := range sm.Groups {
		for _, server := range group.Servers {
			if server.ID == serverID {
				return &server, nil
			}
		}
	}
	return nil, fmt.Errorf("未找到ID为 %s 的服务器", serverID)
}
