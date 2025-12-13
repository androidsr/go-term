package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"go-term/models"

	"golang.org/x/crypto/scrypt"
)

// EncryptedConfigManager 加密配置管理器
type EncryptedConfigManager struct {
	password []byte
}

// NewEncryptedConfigManager 创建新的加密配置管理器
func NewEncryptedConfigManager(password string) *EncryptedConfigManager {
	return &EncryptedConfigManager{
		password: []byte(password),
	}
}

// deriveKey 使用scrypt从密码派生密钥
func (ecm *EncryptedConfigManager) deriveKey(salt []byte) ([]byte, error) {
	return scrypt.Key(ecm.password, salt, 32768, 8, 1, 32)
}

// encrypt 加密数据
func (ecm *EncryptedConfigManager) encrypt(plaintext []byte) (string, error) {
	// 生成随机盐值
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// 派生密钥
	key, err := ecm.deriveKey(salt)
	if err != nil {
		return "", err
	}

	// 创建AES加密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// 生成随机IV
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// 加密数据
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// 将盐值、加密数据组合并进行base64编码
	result := make([]byte, 16+len(ciphertext))
	copy(result[:16], salt)
	copy(result[16:], ciphertext)

	return base64.StdEncoding.EncodeToString(result), nil
}

// decrypt 解密数据
func (ecm *EncryptedConfigManager) decrypt(encryptedData string) ([]byte, error) {
	// base64解码
	data, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	if len(data) < 16 {
		return nil, fmt.Errorf("无效的加密数据")
	}

	// 提取盐值和加密数据
	salt := data[:16]
	ciphertext := data[16:]

	// 派生密钥
	key, err := ecm.deriveKey(salt)
	if err != nil {
		return nil, err
	}

	// 创建AES解密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 创建GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 提取nonce和实际密文
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("密文太短")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// SaveEncryptedConfig 保存加密的配置文件
func (ecm *EncryptedConfigManager) SaveEncryptedConfig(config *models.ServerGroup, filename string) error {
	// 序列化配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("无法序列化配置: %v", err)
	}

	// 加密数据
	encryptedData, err := ecm.encrypt(data)
	if err != nil {
		return fmt.Errorf("加密配置失败: %v", err)
	}

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	// 写入加密数据到文件
	err = os.WriteFile(filename, []byte(encryptedData), 0600)
	if err != nil {
		return fmt.Errorf("无法写入加密配置文件: %v", err)
	}

	return nil
}

// LoadEncryptedConfig 加载加密的配置文件
func (ecm *EncryptedConfigManager) LoadEncryptedConfig(filename string) (*models.ServerGroup, error) {
	// 读取加密文件
	encryptedData, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("无法读取加密配置文件: %v", err)
	}

	// 解密数据
	plaintext, err := ecm.decrypt(string(encryptedData))
	if err != nil {
		return nil, fmt.Errorf("解密配置失败: %v", err)
	}

	// 反序列化配置
	var config models.ServerGroup
	err = json.Unmarshal(plaintext, &config)
	if err != nil {
		return nil, fmt.Errorf("无法解析配置: %v", err)
	}

	return &config, nil
}

// SaveEncryptedServerManager 保存加密的服务器管理器配置
func (ecm *EncryptedConfigManager) SaveEncryptedServerManager(sm *ServerManager, filename string) error {
	// 序列化配置
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return fmt.Errorf("无法序列化服务器管理器: %v", err)
	}

	// 加密数据
	encryptedData, err := ecm.encrypt(data)
	if err != nil {
		return fmt.Errorf("加密服务器管理器失败: %v", err)
	}

	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("无法创建目录: %v", err)
	}

	// 写入加密数据到文件
	err = ioutil.WriteFile(filename, []byte(encryptedData), 0600)
	if err != nil {
		return fmt.Errorf("无法写入加密服务器管理器文件: %v", err)
	}

	return nil
}

// LoadEncryptedServerManager 加载加密的服务器管理器配置
func (ecm *EncryptedConfigManager) LoadEncryptedServerManager(filename string) (*ServerManager, error) {
	// 读取加密文件
	encryptedData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("无法读取加密服务器管理器文件: %v", err)
	}

	// 解密数据
	plaintext, err := ecm.decrypt(string(encryptedData))
	if err != nil {
		return nil, fmt.Errorf("解密服务器管理器失败: %v", err)
	}

	// 反序列化配置
	var sm ServerManager
	err = json.Unmarshal(plaintext, &sm)
	if err != nil {
		return nil, fmt.Errorf("无法解析服务器管理器: %v", err)
	}

	return &sm, nil
}
