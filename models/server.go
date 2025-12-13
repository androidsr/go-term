package models

// ServerGroup 服务器分组
type ServerGroup struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Servers  []Server `json:"servers"`
}

// Server 服务器信息
type Server struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	KeyFile  string `json:"keyFile"` // SSH密钥文件路径
	GroupID  string `json:"groupId"`
}