package controllers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"go-term/models"
	"go-term/services"
)

// AIController AI 能力控制器：提供配置管理、连通性测试与基于 SSH 会话的 AI 运维任务
type AIController struct {
	app      *application.App
	cfgMgr   *services.AIConfigManager
	ssh      *SSHController // 复用已建立的 SSH 连接

	mu         sync.Mutex
	agents     map[string]*services.AIAgent // taskID -> 运行中的 Agent
	taskSeq    uint64
	taskSever  map[string]string                 // taskID -> serverID
	taskMemory map[string][]services.AITaskRecord // serverID -> 历史任务记忆
}

// NewAIController 创建 AI 控制器
func NewAIController(app *application.App, ssh *SSHController) *AIController {
	return &AIController{
		app:        app,
		cfgMgr:     services.NewAIConfigManager(),
		ssh:        ssh,
		agents:     make(map[string]*services.AIAgent),
		taskSever:  make(map[string]string),
		taskMemory: make(map[string][]services.AITaskRecord),
	}
}

// aiConfigPath 解析 AI 配置文件路径：优先可执行文件所在目录的 config/，
// 其次当前工作目录的 config/（与 servers.dat 的查找策略保持一致）
func (ac *AIController) aiConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "config")
		if _, err := os.Stat(dir); err == nil {
			return filepath.Join(dir, "ai.json")
		}
	}
	return filepath.Join("config", "ai.json")
}

// ServiceStartup 应用启动时加载 AI 配置
func (ac *AIController) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	ac.cfgMgr.Load(ac.aiConfigPath())
	return nil
}

// ServiceShutdown 应用关闭时保存配置并停止所有任务
func (ac *AIController) ServiceShutdown(ctx context.Context, options application.ServiceOptions) error {
	_ = ac.cfgMgr.Save(ac.aiConfigPath())
	ac.StopAllAITasks()
	return nil
}

// GetAIConfig 获取 AI 配置
func (ac *AIController) GetAIConfig() services.AIConfig {
	return ac.cfgMgr.Get()
}

// SaveAIConfig 保存 AI 配置（持久化失败会返回具体错误，前端有提示）
func (ac *AIController) SaveAIConfig(cfg services.AIConfig) error {
	ac.cfgMgr.Set(cfg)
	return ac.cfgMgr.Save(ac.aiConfigPath())
}

// TestAIConnection 测试大模型接口连通性
func (ac *AIController) TestAIConnection(cfg services.AIConfig) (string, error) {
	client := services.NewAIClient(cfg)
	reply, err := client.ChatTest()
	if err != nil {
		return "", err
	}
	if reply == "" {
		reply = "(接口返回为空)"
	}
	return reply, nil
}

// aiRunner 将 SSHController 的执行能力适配为 AICommandRunner
type aiRunner struct {
	ac       *AIController
	serverID string
}

// Run 实现 services.AICommandRunner
func (r *aiRunner) Run(command string, onChunk func(string), timeout time.Duration, stop <-chan struct{}) (string, int, error) {
	conn := r.ac.ssh.GetConnection(r.serverID)
	if conn == nil || conn.Client == nil {
		return "", -1, fmt.Errorf("服务器未连接或连接已断开")
	}
	return conn.ExecuteCommandStream(command, onChunk, timeout, stop)
}

// StartAITask 启动一个 AI 运维任务（异步执行，事件通过 ai-event 推送）
// 返回 taskID
func (ac *AIController) StartAITask(serverID, task string) (string, error) {
	cfg := ac.cfgMgr.Get()
	if !cfg.Enabled {
		return "", fmt.Errorf("AI 功能未启用，请先在设置中开启并配置大模型接口")
	}
	task = strings.TrimSpace(task)
	if task == "" {
		return "", fmt.Errorf("请输入任务描述")
	}

	// 校验服务器连接
	conn := ac.ssh.GetConnection(serverID)
	if conn == nil || conn.Client == nil {
		return "", fmt.Errorf("服务器未连接，请先连接服务器")
	}

	// 不允许同一服务器并行多个任务
	ac.mu.Lock()
	for tid, sid := range ac.taskSever {
		if sid == serverID {
			if _, running := ac.agents[tid]; running {
				ac.mu.Unlock()
				return "", fmt.Errorf("该服务器已有正在运行的 AI 任务，请先停止或等待完成")
			}
		}
	}
	ac.taskSeq++
	taskID := fmt.Sprintf("ai_%s_%d", serverID, ac.taskSeq)

	// 组装该服务器的历史任务记忆（最近 MemoryRounds 轮）
	memRounds := cfg.MemoryRounds
	var memText string
	if memRounds > 0 {
		recs := ac.taskMemory[serverID]
		if len(recs) > memRounds {
			recs = recs[len(recs)-memRounds:]
		}
		memText = services.BuildMemoryText(recs)
	}
	ac.mu.Unlock()

	agent := services.NewAIAgent(taskID, cfg, &aiRunner{ac: ac, serverID: serverID}, func(event string, data interface{}) {
		if ac.app != nil {
			ac.app.Event.Emit(event, data)
		}
	}, memText)
	ac.mu.Lock()
	ac.agents[taskID] = agent
	ac.taskSever[taskID] = serverID
	ac.mu.Unlock()

	go func() {
		defer func() {
			ac.mu.Lock()
			delete(ac.agents, taskID)
			delete(ac.taskSever, taskID)
			ac.mu.Unlock()
		}()
		// start 事件带上实际使用的接口与模型，便于前端诊断配置是否正确加载
		ac.app.Event.Emit("ai-event", map[string]interface{}{
			"taskID": taskID, "type": "start", "serverID": serverID, "task": task,
			"endpoint": cfg.BaseURL, "model": cfg.Model,
		})
		agent.Run(task)

		// 任务结束后写入该服务器的记忆（受轮数上限约束）
		if memRounds > 0 {
			rec := agent.Record(task)
			ac.mu.Lock()
			m := append(ac.taskMemory[serverID], rec)
			if len(m) > memRounds {
				m = m[len(m)-memRounds:]
			}
			ac.taskMemory[serverID] = m
			ac.mu.Unlock()
		}
	}()

	return taskID, nil
}

// ClearAIMemory 清空指定服务器的历史任务记忆
func (ac *AIController) ClearAIMemory(serverID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	delete(ac.taskMemory, serverID)
}

// StopAITask 停止指定 AI 任务
func (ac *AIController) StopAITask(taskID string) error {
	ac.mu.Lock()
	agent, ok := ac.agents[taskID]
	ac.mu.Unlock()
	if !ok {
		return fmt.Errorf("任务不存在或已结束")
	}
	agent.Stop()
	return nil
}

// StopAllAITasks 停止全部 AI 任务
func (ac *AIController) StopAllAITasks() {
	ac.mu.Lock()
	agents := make([]*services.AIAgent, 0, len(ac.agents))
	for _, a := range ac.agents {
		agents = append(agents, a)
	}
	ac.mu.Unlock()
	for _, a := range agents {
		a.Stop()
	}
}

// GetAIConfiguredServers 获取已连接服务器列表（供前端选择 AI 任务目标）
func (ac *AIController) GetAIConfiguredServers() []models.Server {
	return ac.ssh.GetAllConnectedServers()
}
