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
	Note     string `json:"note"`   // 备注信息
}

// BatchScript 批量脚本
type BatchScript struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`        // 脚本名称
	Description string   `json:"description"` // 脚本描述
	Content     string   `json:"content"`     // 脚本内容
	ServerIDs   []string `json:"serverIds"`   // 目标服务器ID列表
	ExecutionType string `json:"executionType"` // 执行类型: "script"(脚本模式), "command"(命令模式)
	CreatedAt   string   `json:"createdAt"`   // 创建时间
	UpdatedAt   string   `json:"updatedAt"`   // 更新时间
}

// ScriptExecution 脚本执行记录
type ScriptExecution struct {
	ID         string `json:"id"`
	ScriptID   string `json:"scriptId"`   // 脚本ID
	ServerID   string `json:"serverId"`   // 服务器ID
	ServerName string `json:"serverName"` // 服务器名称
	Status     string `json:"status"`     // 执行状态: pending, running, success, failed
	Output     string `json:"output"`     // 执行输出
	Error      string `json:"error"`      // 错误信息
	StartTime  string `json:"startTime"`  // 开始时间
	EndTime    string `json:"endTime"`    // 结束时间
	CommandOutputs []CommandOutput `json:"commandOutputs"` // 分命令的执行结果
}

// CommandOutput 单个命令的执行结果
type CommandOutput struct {
	Command   string `json:"command"`   // 命令内容
	Output    string `json:"output"`    // 命令输出
	Error     string `json:"error"`     // 命令错误
	Status    string `json:"status"`    // 执行状态: success, failed
	StartTime string `json:"startTime"` // 开始时间
	EndTime   string `json:"endTime"`   // 结束时间
}