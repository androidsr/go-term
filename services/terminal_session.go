package services

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type TerminalSession struct {
	Session *ssh.Session
	Stdin   io.WriteCloser
	stdout  io.Reader
	stderr  io.Reader

	OutputChan chan []byte
	ErrorChan  chan []byte
	closeChan  chan struct{}
	closeOnce  sync.Once

	// 添加一个缓冲区来存储最近的输出，用于处理自动补全等场景
	outputBuffer []byte
	bufferMutex  sync.Mutex

	width  int
	height int
}

func (s *SSHConnection) CreateTerminalSession(width, height int) (*TerminalSession, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("SSH连接未建立")
	}

	session, err := s.Client.NewSession()
	if err != nil {
		return nil, err
	}

	// 如果没有提供宽度和高度，则使用默认值
	if width <= 0 || height <= 0 {
		width = 80
		height = 24
	}

	if err := session.RequestPty("xterm", height, width, ssh.TerminalModes{}); err != nil {
		session.Close()
		return nil, err
	}

	stdin, _ := session.StdinPipe()
	stdout, _ := session.StdoutPipe()
	stderr, _ := session.StderrPipe()

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, err
	}

	ts := &TerminalSession{
		Session:    session,
		Stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		OutputChan: make(chan []byte, 200), // 适中的缓冲区大小，平衡内存和性能
		ErrorChan:  make(chan []byte, 100),
		closeChan:  make(chan struct{}),
		width:      width,
		height:     height,
	}

	// 启动后台读协程
	go ts.readLoop(ts.stdout, ts.OutputChan)
	go ts.readLoop(ts.stderr, ts.ErrorChan)

	return ts, nil
}

func (ts *TerminalSession) readLoop(r io.Reader, out chan []byte) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ts.closeChan:
			return
		default:
			n, err := r.Read(buf)
			if n > 0 {
				// 必须复制，否则 buf 复用导致数据错乱
				data := make([]byte, n)
				copy(data, buf[:n])
				// 检查通道是否已关闭，使用非阻塞发送避免在高输出时阻塞
				select {
				case out <- data:
				case <-ts.closeChan:
					return
				default:
					// 如果通道满了，丢弃最旧的数据为新数据腾出空间
					// 这样可以确保 tail -f 等高输出命令不会阻塞整个终端
					select {
					case <-out: // 丢弃一个旧数据
						out <- data // 发送新数据
					case <-ts.closeChan:
						return
					default:
						// 如果还是发不出去，直接丢弃这个数据包
						// 这比阻塞整个读取循环要好
					}
				}

				// 同时更新输出缓冲区，用于处理自动补全等场景
				ts.bufferMutex.Lock()
				ts.outputBuffer = append(ts.outputBuffer, data...)
				// 限制缓冲区大小，防止内存泄漏
				if len(ts.outputBuffer) > 8192 {
					ts.outputBuffer = ts.outputBuffer[len(ts.outputBuffer)-8192:]
				}
				ts.bufferMutex.Unlock()
			}
			// EOF错误表示连接已正常关闭，可以直接返回
			if err == io.EOF {
				return
			}
			if err != nil {
				// 其他错误记录日志但继续运行
				// 使用fmt.Println代替log.Printf避免导入问题
				fmt.Printf("终端读取错误: %v\n", err)
				return
			}
		}
	}
}

// GetLastOutput 获取最近的输出内容
func (ts *TerminalSession) GetLastOutput() string {
	ts.bufferMutex.Lock()
	defer ts.bufferMutex.Unlock()

	// 返回最后512个字节的内容，足够处理大多数自动补全场景
	start := 0
	if len(ts.outputBuffer) > 512 {
		start = len(ts.outputBuffer) - 512
	}

	return string(ts.outputBuffer[start:])
}

// ClearOutputBuffer 清空输出缓冲区
func (ts *TerminalSession) ClearOutputBuffer() {
	ts.bufferMutex.Lock()
	defer ts.bufferMutex.Unlock()
	ts.outputBuffer = []byte{}
}

// ParseAutoCompleteSuggestions 解析自动补全建议列表
func (ts *TerminalSession) ParseAutoCompleteSuggestions(partialCommand, output string) []string {
	if output == "" {
		return nil
	}

	// 清理输出，移除ANSI转义序列
	cleanOutput := removeANSIEscapeSequences(output)

	// 按行分割输出
	lines := strings.Split(cleanOutput, "\n")

	var suggestions []string

	// 获取最后一个参数（用于路径补全）
	parts := strings.Fields(partialCommand)
	var lastArg string
	var commandPrefix string

	if len(parts) > 0 {
		commandPrefix = strings.Join(parts[:len(parts)-1], " ")
		if len(parts) > 1 {
			commandPrefix += " "
		}
		lastArg = parts[len(parts)-1]
	}

	// 查找补全建议
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// 跳过错误信息和提示符
		if strings.Contains(line, "command not found") ||
			strings.Contains(line, "No such file or directory") ||
			strings.Contains(line, "Last login") ||
			strings.Contains(line, "$ ") || strings.Contains(line, "# ") ||
			strings.Contains(line, "Display all") ||
			strings.HasPrefix(line, partialCommand) && strings.Contains(line, "\x07") {
			continue
		}

		// 如果这一行以我们输入的命令开头，可能是shell回显，跳过
		if strings.HasPrefix(line, partialCommand) && len(line) > len(partialCommand) {
			// 检查是否是补全结果（通常会有特殊的格式）
			if !strings.Contains(line, " ") || line == partialCommand {
				continue
			}
		}

		// 处理补全建议
		if lastArg != "" {
			// 路径补全的情况
			words := strings.Fields(line)
			for _, word := range words {
				word = strings.TrimSpace(word)
				if word == "" {
					continue
				}

				// 检查是否包含我们要补全的部分
				if strings.HasPrefix(word, lastArg) {
					// 构造完整的建议：命令前缀 + 补全后的路径
					fullSuggestion := commandPrefix + word
					suggestions = append(suggestions, fullSuggestion)
				}
			}
		} else {
			// 命令补全的情况
			words := strings.Fields(line)
			for _, word := range words {
				word = strings.TrimSpace(word)
				if word != "" && !strings.Contains(word, "/") {
					suggestions = append(suggestions, word)
				}
			}
		}
	}

	// 如果没有找到有效的建议，尝试其他解析方法
	if len(suggestions) == 0 && lastArg != "" {
		// 尝试直接从输出中提取路径
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line != "" && !strings.Contains(line, "$ ") && !strings.Contains(line, "# ") {
				// 检查是否包含路径分隔符
				if strings.Contains(line, "/") {
					// 提取可能的路径
					paths := strings.Fields(line)
					for _, path := range paths {
						if strings.HasPrefix(path, lastArg) {
							fullSuggestion := commandPrefix + path
							suggestions = append(suggestions, fullSuggestion)
						}
					}
				}
			}
		}
	}

	// 去重并返回
	return removeDuplicates(suggestions)
}

// removeDuplicates 移除重复项
func removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string
	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}
	return result
}

// removeANSIEscapeSequences 移除ANSI转义序列
func removeANSIEscapeSequences(text string) string {
	// 移除ANSI颜色码和控制字符
	result := text

	// 移除 \x1b[...m 格式的ANSI转义序列
	re := strings.NewReplacer(
		"\x1b[0m", "",
		"\x1b[1m", "",
		"\x1b[31m", "",
		"\x1b[32m", "",
		"\x1b[33m", "",
		"\x1b[34m", "",
		"\x1b[35m", "",
		"\x1b[36m", "",
		"\x1b[37m", "",
		"\x1b[1;31m", "",
		"\x1b[1;32m", "",
		"\x1b[1;33m", "",
		"\x1b[1;34m", "",
		"\x1b[1;35m", "",
		"\x1b[1;36m", "",
		"\x1b[1;37m", "",
		"\x07", "", // Bell character
		"\r", "", // Carriage return
	)

	result = re.Replace(result)

	// 移除其他ANSI转义序列（更通用的方法）
	for strings.Contains(result, "\x1b[") {
		start := strings.Index(result, "\x1b[")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "m")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+end+1:]
	}

	return result
}

func (ts *TerminalSession) SendCommand(c string) error {
	// Tab字符特殊处理 - 不添加换行符
	if c == "\t" {
		_, err := ts.Stdin.Write([]byte(c))
		return err
	}
	// 对于包含Tab字符的命令，发送命令部分和Tab字符（不添加换行符）
	if strings.Contains(c, "\t") {
		_, err := ts.Stdin.Write([]byte(c))
		return err
	}
	// 普通命令添加换行符
	_, err := ts.Stdin.Write([]byte(c + "\n"))
	return err
}

// SendCommandWithoutNewline 发送命令但不添加换行符
func (ts *TerminalSession) SendCommandWithoutNewline(c string) error {
	_, err := ts.Stdin.Write([]byte(c))
	return err
}

func (ts *TerminalSession) ReadOutput() (string, error) {
	select {
	case d := <-ts.OutputChan:
		return string(d), nil
	default:
		return "", nil
	}
}

// ResizeTerminal 调整终端大小
func (ts *TerminalSession) ResizeTerminal(width, height int) error {
	if ts.Session == nil {
		return fmt.Errorf("终端会话未建立")
	}

	// 更新本地记录的尺寸
	ts.width = width
	ts.height = height

	// 发送窗口大小调整请求到远程
	return ts.Session.WindowChange(height, width)
}

func (ts *TerminalSession) Close() error {
	var err error
	ts.closeOnce.Do(func() {
		// 先关闭channel，通知readLoop退出
		close(ts.closeChan)

		// 设置一个超时上下文确保不会无限等待
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		// 并行关闭stdin和session，避免顺序依赖导致的死锁
		errCh := make(chan error, 2)

		go func() {
			// stdin关闭可能会返回EOF错误，这在连接已断开时是正常的
			err := ts.Stdin.Close()
			if err != nil && err != io.EOF {
				errCh <- err
			} else {
				errCh <- nil
			}
		}()

		go func() {
			// session关闭可能会返回EOF错误，这在连接已断开时是正常的
			err := ts.Session.Close()
			if err != nil && err != io.EOF {
				errCh <- err
			} else {
				errCh <- nil
			}
		}()

		// 等待两个关闭操作完成或超时
		for i := 0; i < 2; i++ {
			select {
			case closeErr := <-errCh:
				if closeErr != nil && err == nil {
					err = closeErr
				}
			case <-ctx.Done():
				err = fmt.Errorf("terminal session close timeout")
				return
			}
		}
	})
	return err
}
