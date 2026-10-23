package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// ChatMessage 对话消息
type ChatMessage struct {
	Role    string `json:"role"`    // system / user / assistant
	Content string `json:"content"` // 消息内容
}

// capturingTransport 记录最近一次底层传输错误，用于把 LangChainGo
// 吞掉的通用错误（"network error: failed to reach API server"）还原成真实原因
type capturingTransport struct {
	base    http.RoundTripper
	mu      sync.Mutex
	lastErr error
}

func (t *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		t.mu.Lock()
		t.lastErr = err
		t.mu.Unlock()
	}
	return resp, err
}

func (t *capturingTransport) takeErr() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastErr
}

// AIClient 大模型客户端（基于 LangChainGo，兼容 OpenAI 格式接口）
type AIClient struct {
	cfg AIConfig
}

// NewAIClient 创建 AI 客户端
func NewAIClient(cfg AIConfig) *AIClient {
	return &AIClient{cfg: cfg}
}

// validate 校验配置
func (c *AIClient) validate() error {
	if strings.TrimSpace(c.cfg.BaseURL) == "" {
		return fmt.Errorf("接口地址(BaseURL)不能为空")
	}
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return fmt.Errorf("API Key 不能为空")
	}
	if strings.TrimSpace(c.cfg.Model) == "" {
		return fmt.Errorf("模型名称不能为空")
	}
	return nil
}

// normalizeBaseURL 规范化接口地址。
// 用户提供根域名时自动补 /v1，如 https://api.openai.com -> https://api.openai.com/v1
func normalizeBaseURL(raw string) string {
	u := strings.TrimSpace(raw)
	u = strings.TrimRight(u, "/")
	if u != "" && !strings.HasSuffix(u, "/v1") && !strings.HasSuffix(u, "/v2") &&
		!strings.HasSuffix(u, "/v3") && !strings.HasSuffix(u, "/v4") {
		u += "/v1"
	}
	return u
}

// newTransport 构造 HTTP 传输层：
// - useProxy=false 时强制直连（系统设置了 http_proxy 时访问内网网关会被代理拦截导致连接失败）
// - useProxy=true 时遵循系统代理环境变量
func newTransport(useProxy bool) *capturingTransport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	if useProxy {
		base.Proxy = http.ProxyFromEnvironment
	} else {
		base.Proxy = nil // 直连
	}
	return &capturingTransport{base: base}
}

// newLLM 创建 LangChainGo OpenAI 兼容 LLM 实例，返回实例与错误捕获传输层
func (c *AIClient) newLLM() (*openai.LLM, *capturingTransport, error) {
	if err := c.validate(); err != nil {
		return nil, nil, err
	}

	transport := newTransport(c.cfg.UseProxy)
	opts := []openai.Option{
		openai.WithToken(c.cfg.APIKey),
		openai.WithModel(c.cfg.Model),
		openai.WithBaseURL(normalizeBaseURL(c.cfg.BaseURL)),
		openai.WithHTTPClient(&http.Client{Transport: transport, Timeout: 10 * time.Minute}),
	}

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("初始化模型客户端失败: %v", err)
	}
	return llm, transport, nil
}

// toLLMMessages 将通用消息转换为 LangChainGo 消息
func toLLMMessages(messages []ChatMessage) []llms.MessageContent {
	result := make([]llms.MessageContent, 0, len(messages))
	for _, m := range messages {
		var mt llms.ChatMessageType
		switch strings.ToLower(m.Role) {
		case "system":
			mt = llms.ChatMessageTypeSystem
		case "assistant", "ai":
			mt = llms.ChatMessageTypeAI
		default:
			mt = llms.ChatMessageTypeHuman
		}
		result = append(result, llms.TextParts(mt, m.Content))
	}
	return result
}

// wrapCallError 把调用错误转换为可诊断的信息：
// 还原被 LangChainGo 掩盖的真实网络错误，并附上实际请求的接口地址
func wrapCallError(err error, transport *capturingTransport, endpoint string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "failed to reach API server") {
		if real := transport.takeErr(); real != nil {
			return fmt.Errorf("无法连接接口 %s: %v（请检查服务是否在线、地址是否正确；若接口需通过代理访问，请在设置中开启\"使用系统代理\"）", endpoint, real)
		}
		return fmt.Errorf("无法连接接口 %s（请检查服务是否在线、地址是否正确）", endpoint)
	}
	return fmt.Errorf("%v（接口: %s）", err, endpoint)
}

// ChatStream 流式对话：每收到一段增量文本调用 onDelta；返回完整回复文本
func (c *AIClient) ChatStream(messages []ChatMessage, onDelta func(text string)) (string, error) {
	return c.ChatStreamWithStop(messages, onDelta, nil)
}

// ChatStreamWithStop 流式对话，支持外部停止：stop 通道关闭时立即中断请求
func (c *AIClient) ChatStreamWithStop(messages []ChatMessage, onDelta func(text string), stop <-chan struct{}) (string, error) {
	llm, transport, err := c.newLLM()
	if err != nil {
		return "", err
	}
	endpoint := normalizeBaseURL(c.cfg.BaseURL) + "/chat/completions"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if stop != nil {
		go func() {
			select {
			case <-stop:
				cancel() // 立即中断底层 HTTP 请求
			case <-ctx.Done():
			}
		}()
	}

	callOpts := []llms.CallOption{
		llms.WithMaxTokens(c.cfg.MaxTokens),
	}
	if c.cfg.Temperature > 0 {
		callOpts = append(callOpts, llms.WithTemperature(c.cfg.Temperature))
	}
	if onDelta != nil {
		callOpts = append(callOpts, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			onDelta(string(chunk))
			return nil
		}))
	}

	resp, err := llm.GenerateContent(ctx, toLLMMessages(messages), callOpts...)
	if err != nil {
		// 用户主动停止导致的取消不算错误
		if ctx.Err() == context.Canceled || strings.Contains(err.Error(), "context canceled") {
			return "", fmt.Errorf("任务已被用户停止")
		}
		return "", wrapCallError(err, transport, endpoint)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("接口未返回任何内容，请检查模型名称与接口配置（接口: %s）", endpoint)
	}

	content := strings.TrimSpace(resp.Choices[0].Content)
	if content == "" {
		return "", fmt.Errorf("接口返回内容为空，请检查模型名称是否正确（如 deepseek-chat、gpt-4o-mini、qwen-plus）")
	}
	return content, nil
}

// ChatTest 测试连通性：与真实任务使用完全相同的流式链路
func (c *AIClient) ChatTest() (string, error) {
	testCfg := c.cfg
	testCfg.MaxTokens = 64
	client := &AIClient{cfg: testCfg}

	msgs := []ChatMessage{
		{Role: "user", Content: "请只回复两个字：正常"},
	}

	var sb strings.Builder
	var err error
	done := make(chan struct{})
	go func() {
		// 与任务路径一致：启用流式回调
		_, e := client.ChatStream(msgs, func(delta string) {
			sb.WriteString(delta)
		})
		err = e
		close(done)
	}()
	select {
	case <-done:
		return sb.String(), err
	case <-time.After(60 * time.Second):
		return "", fmt.Errorf("连接测试超时（60秒），请检查网络或接口地址")
	}
}
