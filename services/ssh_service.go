package services

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// FileInfo 文件信息
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
	Type  string `json:"type"` // "file" 或 "dir"
}

// SSHConnection SSH连接信息
type SSHConnection struct {
	Client *ssh.Client
}

// Connect 建立SSH连接
func (s *SSHConnection) Connect(host string, port int, username string, password string, keyFile string) error {
	var auth []ssh.AuthMethod

	if keyFile != "" {
		// 使用私钥认证
		key, err := ioutil.ReadFile(keyFile)
		if err != nil {
			return fmt.Errorf("无法读取密钥文件: %v", err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return fmt.Errorf("无法解析私钥: %v", err)
		}

		auth = append(auth, ssh.PublicKeys(signer))
	} else {
		// 使用密码认证
		auth = append(auth, ssh.Password(password))
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 在生产环境中应该使用更安全的主机密钥验证
		Timeout:         30 * time.Second,
	}

	address := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return fmt.Errorf("无法连接到服务器: %v", err)
	}

	s.Client = client
	return nil
}

// ExecuteCommand 执行远程命令
func (s *SSHConnection) ExecuteCommand(command string) (string, error) {
	if s.Client == nil {
		return "", fmt.Errorf("SSH连接未建立")
	}

	session, err := s.Client.NewSession()
	if err != nil {
		return "", fmt.Errorf("无法创建会话: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		// 返回错误信息时同时返回输出内容，以便前端能看到错误详情
		return string(output), fmt.Errorf("执行命令失败: %v", err)
	}

	return string(output), nil
}

// Close 关闭SSH连接
func (s *SSHConnection) Close() {
	if s.Client != nil {
		s.Client.Close()
		s.Client = nil
	}
}

// SFTPConnection SFTP连接信息
type SFTPConnection struct {
	Client *sftp.Client
}

// CreateSFTPClient 创建SFTP客户端
func (s *SSHConnection) CreateSFTPClient() (*sftp.Client, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("SSH连接未建立")
	}

	client, err := sftp.NewClient(s.Client)
	if err != nil {
		return nil, fmt.Errorf("无法创建SFTP客户端: %v", err)
	}

	return client, nil
}

// UploadFile 上传文件
func (s *SSHConnection) UploadFile(sftpClient *sftp.Client, localPath, remotePath string) error {
	if s.Client == nil {
		return fmt.Errorf("SSH连接未建立")
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("无法打开本地文件: %v", err)
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("无法创建远程文件: %v", err)
	}
	defer remoteFile.Close()

	// 使用带缓冲的拷贝，提高大文件传输效率
	buf := make([]byte, 32*1024) // 32KB 缓冲区
	_, err = io.CopyBuffer(remoteFile, localFile, buf)
	if err != nil {
		return fmt.Errorf("文件传输失败: %v", err)
	}

	// 确保数据刷新到磁盘
	if err := remoteFile.Sync(); err != nil {
		return fmt.Errorf("刷新远程文件失败: %v", err)
	}

	return nil
}

// DownloadFile 下载文件
func (s *SSHConnection) DownloadFile(sftpClient *sftp.Client, remotePath, localPath string) error {
	if s.Client == nil {
		return fmt.Errorf("SSH连接未建立")
	}

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("无法打开远程文件: %v", err)
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("无法创建本地文件: %v", err)
	}
	defer localFile.Close()

	// 使用带缓冲的拷贝，提高大文件传输效率
	buf := make([]byte, 32*1024) // 32KB 缓冲区
	_, err = io.CopyBuffer(localFile, remoteFile, buf)
	if err != nil {
		return fmt.Errorf("文件传输失败: %v", err)
	}

	// 确保数据刷新到磁盘
	if err := localFile.Sync(); err != nil {
		return fmt.Errorf("刷新本地文件失败: %v", err)
	}

	return nil
}

// ListDirectory 列出目录内容
func (s *SSHConnection) ListDirectory(sftpClient *sftp.Client, path string) ([]FileInfo, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("SSH连接未建立")
	}

	// 列出目录内容
	files, err := sftpClient.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	var result []FileInfo
	for _, file := range files {
		fileInfo := FileInfo{
			Name:  file.Name(),
			Path:  fmt.Sprintf("%s/%s", path, file.Name()),
			Size:  file.Size(),
			Mtime: file.ModTime().Unix(),
		}

		if file.IsDir() {
			fileInfo.Type = "dir"
		} else {
			fileInfo.Type = "file"
		}

		result = append(result, fileInfo)
	}

	return result, nil
}

// CreateDirectory 创建目录
func (s *SSHConnection) CreateDirectory(sftpClient *sftp.Client, path string) error {
	if s.Client == nil {
		return fmt.Errorf("SSH连接未建立")
	}

	// 创建目录
	err := sftpClient.MkdirAll(path)
	if err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	return nil
}

// DeleteFile 删除文件或目录
func (s *SSHConnection) DeleteFile(sftpClient *sftp.Client, path string) error {
	if s.Client == nil {
		return fmt.Errorf("SSH连接未建立")
	}

	// 获取文件信息以确定是文件还是目录
	fileInfo, err := sftpClient.Stat(path)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %v", err)
	}

	if fileInfo.IsDir() {
		// 删除目录（需要先删除目录中的所有内容）
		err = s.removeDirectory(sftpClient, path)
		if err != nil {
			return fmt.Errorf("删除目录失败: %v", err)
		}
	} else {
		// 删除文件
		err = sftpClient.Remove(path)
		if err != nil {
			return fmt.Errorf("删除文件失败: %v", err)
		}
	}

	return nil
}

// removeDirectory 递归删除目录
func (s *SSHConnection) removeDirectory(sftpClient *sftp.Client, path string) error {
	// 列出目录内容
	files, err := sftpClient.ReadDir(path)
	if err != nil {
		return err
	}

	// 删除所有子项
	for _, file := range files {
		filePath := fmt.Sprintf("%s/%s", path, file.Name())
		if file.IsDir() {
			// 递归删除子目录
			err = s.removeDirectory(sftpClient, filePath)
			if err != nil {
				return err
			}
		} else {
			// 删除文件
			err = sftpClient.Remove(filePath)
			if err != nil {
				return err
			}
		}
	}

	// 删除空目录
	return sftpClient.RemoveDirectory(path)
}

// newSessionWithTimeout 在超时时间内尝试创建 session，否则返回错误。
// 如果超时发生，会尝试关闭底层 client 以便外层可以重新建立连接。
func (s *SSHConnection) newSessionWithTimeout(timeout time.Duration) (*ssh.Session, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("SSH connection not established")
	}

	type res struct {
		session *ssh.Session
		err     error
	}

	ch := make(chan res, 1)
	go func() {
		session, err := s.Client.NewSession()
		ch <- res{session: session, err: err}
	}()

	select {
	case r := <-ch:
		return r.session, r.err
	case <-time.After(timeout):
		// 超时：认为当前 underlying client 可能处于不健康状态，强制关闭 client。
		// 上层会收到错误并可以选择重连（Connect）。
		_ = s.Client.Close()
		s.Client = nil
		return nil, fmt.Errorf("NewSession timeout after %v; closed underlying client for recovery", timeout)
	}
}
