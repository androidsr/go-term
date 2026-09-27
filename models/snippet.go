package models

// Snippet 命令片段：保存常用命令，可一键发送到终端。
type Snippet struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	ServerID  string `json:"serverId"` // 可选：关联服务器（为空表示通用）
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
