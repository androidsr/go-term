package models

// OpsConfig 运维配置：一个轻量的配置笔记，集中存放用户自填的运维信息。
// 只保留最简字段：名称（用户自填）、关联项目（服务器）、以及一段自由文本（内容）。
type OpsConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`     // 配置名称，由用户自填，例如 "生产-订单服务"
	ServerID string `json:"serverId"` // 关联的项目/服务器ID，可留空表示通用配置
	Content  string `json:"content"`  // 自由文本内容，由用户在全窗口文本框中输入
	UpdatedAt string `json:"updatedAt"` // 更新时间
}
