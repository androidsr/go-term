package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockOpenAIServer 模拟 OpenAI 兼容 SSE 流式接口
func mockOpenAIServer(t *testing.T, receivedPath *string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if receivedPath != nil {
			*receivedPath = r.URL.Path
		}
		// 校验鉴权头
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		chunks := []string{
			`data: {"choices":[{"delta":{"content":"你"}}]}`,
			`data: {"choices":[{"delta":{"content":"好"}}]}`,
			`data: {"choices":[{"delta":{"content":"，世界"}}]}`,
			`data: [DONE]`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte(c + "\n\n"))
			flusher.Flush()
		}
	}))
}

func TestChatStreamWithMockServer(t *testing.T) {
	var path string
	srv := mockOpenAIServer(t, &path)
	defer srv.Close()

	cfg := AIConfig{
		Enabled:     true,
		BaseURL:     srv.URL, // 不带 /v1，应自动补全
		APIKey:      "test-key",
		Model:       "gpt-4o-mini",
		Temperature: 0.3,
		MaxTokens:   128,
	}
	client := NewAIClient(cfg)

	var deltas []string
	reply, err := client.ChatStream([]ChatMessage{
		{Role: "system", Content: "你是助手"},
		{Role: "user", Content: "打个招呼"},
	}, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("ChatStream 失败: %v", err)
	}
	if reply != "你好，世界" {
		t.Fatalf("回复内容不符: %q", reply)
	}
	if len(deltas) != 3 {
		t.Fatalf("流式回调次数不符: %d", len(deltas))
	}
	// 验证 baseURL 自动补全 /v1 后请求路径正确
	if !strings.HasSuffix(path, "/v1/chat/completions") {
		t.Fatalf("请求路径错误: %s", path)
	}
}

func TestChatTestUnauthorized(t *testing.T) {
	srv := mockOpenAIServer(t, nil)
	defer srv.Close()

	cfg := AIConfig{BaseURL: srv.URL, APIKey: "wrong-key", Model: "gpt-4o-mini", MaxTokens: 64}
	client := NewAIClient(cfg)
	_, err := client.ChatTest()
	if err == nil {
		t.Fatal("错误的 API Key 应当返回错误")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com":                          "https://api.openai.com/v1",
		"https://api.openai.com/":                         "https://api.openai.com/v1",
		"https://api.openai.com/v1":                       "https://api.openai.com/v1",
		"https://dashscope.aliyuncs.com/compatible-mode/v1": "https://dashscope.aliyuncs.com/compatible-mode/v1",
		"http://localhost:11434":                          "http://localhost:11434/v1",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}
