package services

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"
)

// AIFileOps AI 文件操作内置工具：
// 读取远端文件 → 在 Go 内做精确修改 → 回写（自动备份原文件、写入后校验一致性）。
// 相比让 AI 直接输出 sed/awk 命令，此方式不受引号/转义/特殊字符影响，稳定精确。
type AIFileOps struct {
	exec AIExecRunner
}

// AIExecRunner 文件工具使用的精确执行接口（始终走后台 exec，不依赖任务模式）
type AIExecRunner interface {
	RunDirect(command string, timeout time.Duration, stop <-chan struct{}) (string, error)
}

const aiFileMaxSize = 4 * 1024 * 1024 // 4MB，超过则拒绝编辑

// shellQuote 单引号转义，保证路径/参数按字面量传给 shell
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Read 读取文件内容，返回带行号的文本（供 AI 查看）
func (f *AIFileOps) Read(path string, stop <-chan struct{}) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("文件路径为空")
	}
	out, err := f.exec.RunDirect("cat -n "+shellQuote(path)+" 2>&1 && echo __AI_READ_OK__", 30*time.Second, stop)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	if !strings.Contains(out, "__AI_READ_OK__") {
		return "", fmt.Errorf("读取文件失败（文件不存在或无权限）: %s", path)
	}
	out = strings.ReplaceAll(out, "__AI_READ_OK__", "")
	out = strings.TrimSpace(out)
	if out == "" {
		return "(文件为空)", nil
	}
	// 超长截断，只保留最后 400 行，避免撑爆上下文
	lines := strings.Split(out, "\n")
	const maxLines = 400
	if len(lines) > maxLines {
		out = fmt.Sprintf("…（文件共 %d 行，仅显示最后 %d 行）\n", len(lines), maxLines) + strings.Join(lines[len(lines)-maxLines:], "\n")
	}
	return out, nil
}

// Edit 执行文件编辑，返回给 AI 的结果描述
// op: insert / replace / delete / insert_after_match / replace_match
func (f *AIFileOps) Edit(path, op string, line int, match string, content []string, stop <-chan struct{}) (string, error) {
	path = strings.TrimSpace(path)
	op = strings.ToLower(strings.TrimSpace(op))
	if path == "" {
		return "", fmt.Errorf("文件路径为空")
	}
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("文件路径必须是绝对路径（以 / 开头）: %s", path)
	}

	// 1. 读取原始内容
	raw, err := f.readRaw(path, stop)
	if err != nil {
		return "", err
	}

	// 2. Go 内精确修改
	lines := strings.Split(raw, "\n")
	// cat 输出保留末尾换行时 split 会多一个空元素，去掉
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	newLines, changed, err := applyFileEdit(lines, op, line, match, content)
	if err != nil {
		return "", err
	}
	if !changed {
		return "文件未发生变化（无可应用的修改）", nil
	}

	newRaw := strings.Join(newLines, "\n") + "\n"

	// 3. 回写：备份 → heredoc 写入（带随机分隔符防止内容被 shell 展开/破坏）→ md5 校验
	if err := f.writeRemote(path, newRaw, stop); err != nil {
		return "", err
	}
	return fmt.Sprintf("编辑成功（%s，共 %d 行 → %d 行）。原文件已备份。", op, len(lines), len(newLines)), nil
}

// readRaw 读取文件原始内容
func (f *AIFileOps) readRaw(path string, stop <-chan struct{}) (string, error) {
	out, err := f.exec.RunDirect("wc -c "+shellQuote(path)+" 2>/dev/null && cat "+shellQuote(path)+" 2>/dev/null && echo __AI_CAT_OK__", 30*time.Second, stop)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}
	if !strings.Contains(out, "__AI_CAT_OK__") {
		return "", fmt.Errorf("读取文件失败（文件不存在或无权限）: %s", path)
	}
	// 去掉开头的 wc -c 行
	if idx := strings.Index(out, "\n"); idx >= 0 {
		out = out[idx+1:]
	}
	out = strings.ReplaceAll(out, "__AI_CAT_OK__\n", "")
	out = strings.ReplaceAll(out, "__AI_CAT_OK__", "")
	if len(out) > aiFileMaxSize {
		return "", fmt.Errorf("文件过大（超过 4MB），不支持 AI 编辑: %s", path)
	}
	return out, nil
}

// writeRemote 备份并写回文件，md5 校验写入结果
func (f *AIFileOps) writeRemote(path, content string, stop <-chan struct{}) error {
	delim := "AI_EDIT_EOF_" + fmt.Sprintf("%x", time.Now().UnixNano())
	if strings.Contains(content, delim) {
		return fmt.Errorf("文件内容包含保留分隔符，无法安全写入")
	}
	bakPath := fmt.Sprintf("%s.bak_%s", path, time.Now().Format("20060102_150405"))

	var cmd strings.Builder
	cmd.WriteString("cp -p " + shellQuote(path) + " " + shellQuote(bakPath) + " && ")
	cmd.WriteString("cat > " + shellQuote(path) + " <<'" + delim + "'\n")
	cmd.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		cmd.WriteString("\n")
	}
	cmd.WriteString(delim + "\n")
	cmd.WriteString("echo __AI_WRITE_OK__ && (md5sum " + shellQuote(path) + " 2>/dev/null || true)")

	out, err := f.exec.RunDirect(cmd.String(), 60*time.Second, stop)
	if err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}
	if !strings.Contains(out, "__AI_WRITE_OK__") {
		return fmt.Errorf("写入文件失败（目录不存在或无写权限）: %s", path)
	}

	// 校验：比较 md5
	expect := fmt.Sprintf("%x", md5.Sum([]byte(content)))
	for _, l := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(l))
		if len(fields) >= 2 && fields[1] == path {
			if fields[0] != expect {
				return fmt.Errorf("写入校验失败（md5 不一致），请重新读取文件确认内容")
			}
			return nil
		}
	}
	// 服务器没有 md5sum 时跳过校验
	return nil
}

// applyFileEdit 在内存中应用编辑操作，返回新行集与是否发生变化
func applyFileEdit(lines []string, op string, line int, match string, content []string) ([]string, bool, error) {
	switch op {
	case "insert":
		// 在第 line 行之后插入（line=0 表示插入到文件开头）
		if line < 0 || line > len(lines) {
			return nil, false, fmt.Errorf("insert 的行号 %d 超出范围（文件共 %d 行，0 表示文件开头）", line, len(lines))
		}
		out := make([]string, 0, len(lines)+len(content))
		out = append(out, lines[:line]...)
		out = append(out, content...)
		out = append(out, lines[line:]...)
		return out, true, nil

	case "replace":
		if line < 1 || line > len(lines) {
			return nil, false, fmt.Errorf("replace 的行号 %d 超出范围（1-%d）", line, len(lines))
		}
		out := make([]string, 0, len(lines)+len(content))
		out = append(out, lines[:line-1]...)
		out = append(out, content...)
		out = append(out, lines[line:]...)
		return out, true, nil

	case "delete":
		if match != "" && line <= 0 {
			// 删除所有包含关键字的行
			var out []string
			for _, l := range lines {
				if strings.Contains(l, match) {
					continue
				}
				out = append(out, l)
			}
			return out, len(out) != len(lines), nil
		}
		if line < 1 || line > len(lines) {
			return nil, false, fmt.Errorf("delete 的行号 %d 超出范围（1-%d）", line, len(lines))
		}
		out := append(append([]string{}, lines[:line-1]...), lines[line:]...)
		return out, true, nil

	case "insert_after_match":
		idx := findLine(lines, match)
		if idx < 0 {
			return nil, false, fmt.Errorf("未找到包含关键字 %q 的行", match)
		}
		out := make([]string, 0, len(lines)+len(content))
		out = append(out, lines[:idx+1]...)
		out = append(out, content...)
		out = append(out, lines[idx+1:]...)
		return out, true, nil

	case "replace_match":
		idx := findLine(lines, match)
		if idx < 0 {
			return nil, false, fmt.Errorf("未找到包含关键字 %q 的行", match)
		}
		out := make([]string, 0, len(lines)+len(content))
		out = append(out, lines[:idx]...)
		out = append(out, content...)
		out = append(out, lines[idx+1:]...)
		return out, true, nil

	default:
		return nil, false, fmt.Errorf("不支持的编辑操作 %q（可用: insert/replace/delete/insert_after_match/replace_match）", op)
	}
}

// findLine 返回第一个包含 match 的行下标（0-based），未找到返回 -1
func findLine(lines []string, match string) int {
	if match == "" {
		return -1
	}
	for i, l := range lines {
		if strings.Contains(l, match) {
			return i
		}
	}
	return -1
}
