package models

// OpsConfig 运维配置：集中存放服务器的部署方式、应用目录等信息，
// 避免把这些运维知识散落到各处。
type OpsConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`         // 配置名称，例如 "生产-订单服务"
	ServerID     string `json:"serverId"`     // 关联的服务器ID
	DeployMethod string `json:"deployMethod"` // 部署方式: source(源码编译)/binary(二进制)/docker/docker-compose/k8s/systemd/other
	AppDir       string `json:"appDir"`       // 应用所在目录
	StartCmd     string `json:"startCmd"`     // 启动命令 / 启动脚本路径
	EnvVars      string `json:"envVars"`      // 环境变量（KEY=VALUE 每行一个）
	RepoURL      string `json:"repoUrl"`      // 代码仓库地址（可选）
	LogsDir      string `json:"logsDir"`      // 日志目录（可选）
	Notes        string `json:"notes"`        // 备注（可选）
	UpdatedAt    string `json:"updatedAt"`    // 更新时间
}
