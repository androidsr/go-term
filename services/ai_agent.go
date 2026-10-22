package services

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ============ SSH 命令流式执行（供 AI Agent 使用） ============

// ExecuteCommandStream 在服务器上执行命令并实时回调输出块。
// 返回完整输出与退出码（-1 表示未知）。onChunk 为 nil 时退化为普通执行。
func (s *SSHConnection) ExecuteCommandStream(command string, onChunk func(string), timeout time.Duration, stop <-chan struct{}) (string, int, error) {
	if s.Client == nil {
		return "", -1, fmt.Errorf("SSH连接未建立")
	}

	session, err := s.Client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("无法创建会话: %v", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", -1, fmt.Errorf("获取标准输出失败: %v", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", -1, fmt.Errorf("获取标准错误失败: %v", err)
	}

	var outMu sync.Mutex
	var full strings.Builder
	collect := func(r interface{ Read([]byte) (int, error) }) {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				outMu.Lock()
				full.WriteString(chunk)
				outMu.Unlock()
				if onChunk != nil {
					onChunk(chunk)
				}
			}
			if err != nil {
				return
			}
		}
	}

	done := make(chan error, 1)
	go func() {
		go collect(stdout)
		go collect(stderr)
		done <- session.Start(command)
	}()

	timedOut := false
	stopTimerHit := false
	var timer *time.Timer
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	timer = time.AfterFunc(timeout, func() {
		timedOut = true
		_ = session.Close()
	})

	var startErr error
	select {
	case startErr = <-done:
	case <-stop:
		stopTimerHit = true
		_ = session.Close()
		<-done
	}
	if stopTimerHit {
		outMu.Lock()
		out := full.String()
		outMu.Unlock()
		return out, -1, fmt.Errorf("任务已被用户停止")
	}
	if startErr != nil {
		timer.Stop()
		outMu.Lock()
		out := full.String()
		outMu.Unlock()
		return out, -1, fmt.Errorf("启动命令失败: %v", startErr)
	}

	waitErr := make(chan error, 1)
	go func() { waitErr <- session.Wait() }()

	var werr error
	select {
	case werr = <-waitErr:
	case <-stop:
		stopTimerHit = true
		_ = session.Close()
		<-waitErr
	}
	timer.Stop()

	outMu.Lock()
	out := full.String()
	outMu.Unlock()

	if stopTimerHit {
		return out, -1, fmt.Errorf("任务已被用户停止")
	}
	if timedOut {
		return out, -1, fmt.Errorf("命令执行超时（%s）", timeout)
	}

	exitCode := 0
	if werr != nil {
		// *ssh.ExitError 携带退出状态；其他错误视为执行异常
		if ee, ok := werr.(interface{ ExitStatus() int }); ok {
			exitCode = ee.ExitStatus()
			if exitCode != 0 {
				// 非零退出码不算致命错误，把输出交给 AI 分析
				return out, exitCode, nil
			}
		} else {
			exitCode = -1
			if out == "" {
				return out, exitCode, fmt.Errorf("执行命令失败: %v", werr)
			}
		}
	}
	return out, exitCode, nil
}

// ============ AI Agent（循环式命令执行代理） ============

// AICommandRunner 命令执行器接口（由 controller 提供绑定到具体 server 的执行能力）
type AICommandRunner interface {
	Run(command string, onChunk func(string), timeout time.Duration, stop <-chan struct{}) (string, int, error)
}

// AITaskRecord 单个任务的历史记忆记录
type AITaskRecord struct {
	Task     string   `json:"task"`     // 用户任务描述
	Summary  string   `json:"summary"`  // 最终结论（总结或失败原因）
	Commands []string `json:"commands"` // 执行过的命令
	Success  bool     `json:"success"`  // 是否成功完成
}

// BuildMemoryText 将历史任务记录构建为注入提示词的记忆文本
func BuildMemoryText(records []AITaskRecord) string {
	if len(records) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## 历史任务记忆（本服务器上之前完成的 AI 任务，供参考；已完成的事情不要重复执行，未完成的事情可以继续）\n")
	for i, r := range records {
		status := "已完成"
		if !r.Success {
			status = "未完成"
		}
		sb.WriteString(fmt.Sprintf("[第%d次] 任务: %s\n状态: %s\n结论: %s\n", i+1, r.Task, status, r.Summary))
		if len(r.Commands) > 0 {
			sb.WriteString("执行过的命令: " + strings.Join(r.Commands, " && ") + "\n")
		}
	}
	return sb.String()
}

// AIAgent 单任务 Agent：按 RUN/FINAL 协议循环执行命令直到完成任务
type AIAgent struct {
	client     *AIClient
	runner     AICommandRunner
	cfg        AIConfig
	taskID     string
	emit       func(event string, data interface{})
	stop       chan struct{}
	stopMu     sync.Mutex
	memoryText string    // 注入的历史任务记忆
	executed   []string  // 本次任务执行过的命令
	finalSummary string  // 最终结论
	finalOK    bool      // 任务是否成功完成
}

// NewAIAgent 创建 Agent（memoryText 为该服务器的历史任务记忆，可为空）
func NewAIAgent(taskID string, cfg AIConfig, runner AICommandRunner, emit func(event string, data interface{}), memoryText string) *AIAgent {
	return &AIAgent{
		client:     NewAIClient(cfg),
		runner:     runner,
		cfg:        cfg,
		taskID:     taskID,
		emit:       emit,
		stop:       make(chan struct{}),
		memoryText: memoryText,
	}
}

// Record 生成本次任务的历史记忆记录
func (a *AIAgent) Record(task string) AITaskRecord {
	summary := a.finalSummary
	if summary == "" {
		summary = "任务被用户停止或未产生结论"
	}
	cmds := a.executed
	if len(cmds) > 20 {
		cmds = cmds[len(cmds)-20:]
	}
	return AITaskRecord{Task: task, Summary: summary, Commands: cmds, Success: a.finalOK}
}

// Stop 停止任务
func (a *AIAgent) Stop() {
	a.stopMu.Lock()
	defer a.stopMu.Unlock()
	select {
	case <-a.stop:
	default:
		close(a.stop)
	}
}

// stopped 是否已请求停止
func (a *AIAgent) stopped() bool {
	select {
	case <-a.stop:
		return true
	default:
		return false
	}
}

// emitEvent 推送任务事件
func (a *AIAgent) emitEvent(eventType string, extra map[string]interface{}) {
	if a.emit == nil {
		return
	}
	data := map[string]interface{}{
		"taskID": a.taskID,
		"type":   eventType,
	}
	for k, v := range extra {
		data[k] = v
	}
	a.emit("ai-event", data)
}

// agentSystemPrompt 构造系统提示词
func (a *AIAgent) agentSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString("你是一个资深的 Linux 服务器运维专家，运行在 SSH 终端工具中，可以直接在远程服务器上执行命令来完成用户交给你的运维任务。\n\n")
	sb.WriteString("## 工作协议（必须严格遵守）\n")
	sb.WriteString("你每一轮回复中必须包含且仅包含以下两种指令之一（放在回复的最后一行）：\n")
	sb.WriteString("1. `RUN: <一条shell命令>` —— 表示你需要在服务器上执行这条命令。执行结果会在下一轮返回给你。每次只执行一条命令，遇到需要确认的操作（如 apt/yum 安装）必须使用 -y 等非交互参数，避免交互式卡住。\n")
	sb.WriteString("2. `FINAL: <任务总结>` —— 表示任务已完成或无法继续，给出简明的中文总结（做了什么、结果如何、有哪些注意事项）。\n\n")
	sb.WriteString("## 工作要求\n")
	sb.WriteString("- 先用探测命令了解系统环境（发行版、包管理器、版本、资源等），再决定具体操作方案。\n")
	sb.WriteString("- 安装软件时先检查是否已安装，已安装则直接汇报。\n")
	sb.WriteString("- 每一步执行后根据真实输出决定下一步，命令失败要分析原因并调整方案，最多重试不同方案。\n")
	sb.WriteString("- 不要执行破坏性命令（rm -rf /、mkfs、dd 写磁盘等）。\n")
	sb.WriteString("- 在每条 RUN 指令之前，可以用简短中文说明这一步要做什么。\n")
	sb.WriteString("- 输出必须精炼，不要长篇大论。\n")
	if strings.TrimSpace(a.cfg.SystemHint) != "" {
		sb.WriteString("\n## 用户附加要求\n")
		sb.WriteString(a.cfg.SystemHint)
		sb.WriteString("\n")
	}
	return sb.String()
}

// 指令解析
var (
	runRe   = regexp.MustCompile("(?mi)^\\s*(?:`{3}(?:shell|bash|sh)?\\s*)?RUN\\s*[:\uFF1A]\\s*(.+?)\\s*(?:`{3})?\\s*$")
	finalRe = regexp.MustCompile("(?mi)^\\s*(?:`{3})?FINAL\\s*[:\uFF1A]\\s*(.+)$")
)

type agentDirective struct {
	kind string // run / final / none
	body string
	text string // 指令之外的说明文字
}

// parseDirective 从模型回复中解析指令
func parseDirective(reply string) agentDirective {
	if m := finalRe.FindStringSubmatch(reply); m != nil {
		text := strings.TrimSpace(strings.Replace(reply, m[0], "", 1))
		return agentDirective{kind: "final", body: strings.TrimSpace(m[1]), text: text}
	}
	// 取最后一个 RUN 指令
	matches := runRe.FindAllStringSubmatch(reply, -1)
	if len(matches) > 0 {
		last := matches[len(matches)-1]
		text := strings.TrimSpace(strings.Replace(reply, last[0], "", 1))
		cmd := strings.TrimSpace(last[1])
		cmd = strings.Trim(cmd, "`")
		return agentDirective{kind: "run", body: cmd, text: text}
	}
	return agentDirective{kind: "none", body: "", text: strings.TrimSpace(reply)}
}

// probeEnvironment 探测服务器基础环境信息
func (a *AIAgent) probeEnvironment() string {
	cmds := []string{
		"uname -a",
		"cat /etc/os-release 2>/dev/null | head -5",
		"whoami && id",
		"df -h / 2>/dev/null | tail -2",
		"free -h 2>/dev/null | head -2 || vmstat 2>/dev/null | head -3",
		"nproc",
	}
	var results []string
	for _, c := range cmds {
		out, _, err := a.runner.Run(c, nil, 15*time.Second, a.stop)
		if err != nil && out == "" {
			results = append(results, fmt.Sprintf("$ %s\n(失败: %v)", c, err))
			continue
		}
		results = append(results, fmt.Sprintf("$ %s\n%s", c, strings.TrimSpace(out)))
	}
	return strings.Join(results, "\n\n")
}

// Run 运行 Agent 主循环
func (a *AIAgent) Run(task string) {
	defer a.emitEvent("done", nil)

	if a.stopped() {
		return
	}
	if a.runner == nil {
		a.finalSummary = "服务器未连接，无法执行命令"
		a.emitEvent("error", map[string]interface{}{"message": a.finalSummary})
		return
	}

	a.emitEvent("status", map[string]interface{}{"message": "正在探测服务器环境…"})
	envInfo := a.probeEnvironment()
	if a.stopped() {
		return
	}

	// 注入历史任务记忆（跨任务连续性）
	memorySection := ""
	if strings.TrimSpace(a.memoryText) != "" {
		memorySection = "\n\n" + a.memoryText
	}

	systemMsg := ChatMessage{Role: "system", Content: a.agentSystemPrompt()}
	messages := []ChatMessage{
		systemMsg,
		{Role: "user", Content: fmt.Sprintf("任务：%s\n%s\n\n当前服务器环境信息如下：\n```\n%s\n```\n请开始完成这个任务。", task, memorySection, envInfo)},
	}

	maxSteps := a.cfg.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 25
	}
	failStreak := 0

	for step := 1; step <= maxSteps; step++ {
		if a.stopped() {
			return
		}

		a.emitEvent("status", map[string]interface{}{"message": fmt.Sprintf("AI 思考中（第 %d/%d 轮）…", step, maxSteps)})

		var replyBuf strings.Builder
		reply, err := a.client.ChatStream(messages, func(delta string) {
			replyBuf.WriteString(delta)
			a.emitEvent("delta", map[string]interface{}{"text": delta})
		})
		if err != nil {
			a.finalSummary = "调用大模型接口失败: " + err.Error()
			a.emitEvent("error", map[string]interface{}{"message": a.finalSummary})
			return
		}
		messages = append(messages, ChatMessage{Role: "assistant", Content: reply})

		dir := parseDirective(reply)
		switch dir.kind {
		case "final":
			a.finalSummary = dir.body
			a.finalOK = true
			a.emitEvent("final", map[string]interface{}{"summary": dir.body})
			return
		case "run":
			failStreak = 0
		default:
			failStreak++
			if failStreak >= 3 {
				a.finalSummary = "AI 连续多轮未按协议返回指令，任务中止"
				a.emitEvent("error", map[string]interface{}{"message": a.finalSummary})
				return
			}
			messages = append(messages, ChatMessage{Role: "user", Content: "你的回复不符合协议。请严格在回复最后一行输出 `RUN: <一条命令>` 或 `FINAL: <总结>`。"})
			continue
		}

		if a.stopped() {
			return
		}

		// 执行命令（实时流式回传输出）
		a.executed = append(a.executed, dir.body)
		a.emitEvent("command", map[string]interface{}{"command": dir.body})
		out, exitCode, execErr := a.runner.Run(dir.body, func(chunk string) {
			a.emitEvent("output", map[string]interface{}{"chunk": chunk})
		}, 10*time.Minute, a.stop)

		if a.stopped() {
			return
		}
		a.emitEvent("command-done", map[string]interface{}{
			"command":  dir.body,
			"exitCode": exitCode,
			"error":    errString(execErr),
		})

		// 截断回传给模型的输出，避免撑爆上下文
		feedback := out
		const maxFeedback = 6000
		if len(feedback) > maxFeedback {
			feedback = feedback[len(feedback)-maxFeedback:]
			feedback = "…（前面输出过长已截断，仅保留最后部分）\n" + feedback
		}
		if strings.TrimSpace(feedback) == "" {
			feedback = "(命令没有任何输出)"
		}
		status := fmt.Sprintf("退出码: %d", exitCode)
		if execErr != nil {
			status += fmt.Sprintf("（执行异常: %v）", execErr)
		}
		messages = append(messages, ChatMessage{Role: "user", Content: fmt.Sprintf(
			"命令执行结果：\n$ %s\n%s\n输出：\n```\n%s\n```\n请根据结果继续（成功则继续下一步，失败则分析并调整方案）。", dir.body, status, feedback)})
	}

	a.finalSummary = fmt.Sprintf("已达到最大执行轮次（%d 步），任务中止", maxSteps)
	a.emitEvent("error", map[string]interface{}{"message": a.finalSummary + "。可在总结中查看当前进度"})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
