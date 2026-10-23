package services

import (
	"strings"
	"testing"
)

func TestParseDirectiveRun(t *testing.T) {
	d := parseDirective("先检查一下\nRUN: apt-get install -y nginx")
	if d.kind != "run" || d.body != "apt-get install -y nginx" {
		t.Fatalf("run 解析错误: %+v", d)
	}
}

func TestParseDirectiveFinal(t *testing.T) {
	d := parseDirective("## 完成\nFINAL: nginx 已安装并启动")
	if d.kind != "final" || d.body != "nginx 已安装并启动" {
		t.Fatalf("final 解析错误: %+v", d)
	}
}

func TestParseDirectiveFileRead(t *testing.T) {
	d := parseDirective("查看配置\nFILE_READ: /etc/nginx/nginx.conf")
	if d.kind != "file_read" || d.body != "/etc/nginx/nginx.conf" {
		t.Fatalf("file_read 解析错误: %+v", d)
	}
}

func TestParseDirectiveFileEditInsert(t *testing.T) {
	reply := "插入配置\nFILE_EDIT: /etc/nginx/nginx.conf\nOP: insert\nLINE: 12\nCONTENT:\n    server_name example.com;\n    listen 8080;\nEND_FILE_EDIT"
	d := parseDirective(reply)
	if d.kind != "file_edit" {
		t.Fatalf("kind=%s", d.kind)
	}
	if d.path != "/etc/nginx/nginx.conf" || d.op != "insert" || d.line != 12 {
		t.Fatalf("字段解析错误: %+v", d)
	}
	if len(d.content) != 2 || d.content[0] != "    server_name example.com;" || d.content[1] != "    listen 8080;" {
		t.Fatalf("内容解析错误: %q", d.content)
	}
	if d.text != "插入配置" {
		t.Fatalf("说明文字解析错误: %q", d.text)
	}
}

func TestParseDirectiveFileEditDeleteNoContent(t *testing.T) {
	reply := "FILE_EDIT: /app/config.yml\nOP: delete\nMATCH: debug = true\nEND_FILE_EDIT"
	d := parseDirective(reply)
	if d.kind != "file_edit" || d.op != "delete" || d.match != "debug = true" || len(d.content) != 0 {
		t.Fatalf("解析错误: %+v", d)
	}
}

func TestParseDirectiveFileEditNoEndMarker(t *testing.T) {
	// 流式输出中未闭合的块也要能解析
	reply := "FILE_EDIT: /etc/hosts\nOP: replace_match\nMATCH: old-host\nCONTENT:\n127.0.0.1 new-host"
	d := parseDirective(reply)
	if d.kind != "file_edit" || d.op != "replace_match" || len(d.content) != 1 || d.content[0] != "127.0.0.1 new-host" {
		t.Fatalf("解析错误: %+v", d)
	}
}

func TestParseDirectiveFinalBeatsOthers(t *testing.T) {
	d := parseDirective("FILE_READ: /a\nFINAL: done")
	if d.kind != "final" {
		t.Fatalf("final 应优先: %+v", d)
	}
}

func TestApplyFileEditInsert(t *testing.T) {
	lines := []string{"a", "b", "c"}
	out, changed, err := applyFileEdit(lines, "insert", 1, "", []string{"X", "Y"})
	if err != nil || !changed {
		t.Fatalf("err=%v changed=%v", err, changed)
	}
	if got := strings.Join(out, ","); got != "a,X,Y,b,c" {
		t.Fatalf("结果错误: %s", got)
	}
}

func TestApplyFileEditInsertAtTop(t *testing.T) {
	out, _, err := applyFileEdit([]string{"a", "b"}, "insert", 0, "", []string{"HEAD"})
	if err != nil || strings.Join(out, ",") != "HEAD,a,b" {
		t.Fatalf("err=%v out=%v", err, out)
	}
}

func TestApplyFileEditReplace(t *testing.T) {
	out, _, err := applyFileEdit([]string{"a", "b", "c"}, "replace", 2, "", []string{"B1", "B2"})
	if err != nil || strings.Join(out, ",") != "a,B1,B2,c" {
		t.Fatalf("err=%v out=%v", err, out)
	}
}

func TestApplyFileEditDeleteByLine(t *testing.T) {
	out, _, err := applyFileEdit([]string{"a", "b", "c"}, "delete", 2, "", nil)
	if err != nil || strings.Join(out, ",") != "a,c" {
		t.Fatalf("err=%v out=%v", err, out)
	}
}

func TestApplyFileEditDeleteByMatch(t *testing.T) {
	out, _, err := applyFileEdit([]string{"a", "b=1", "c", "b=2"}, "delete", 0, "b=", nil)
	if err != nil || strings.Join(out, ",") != "a,c" {
		t.Fatalf("err=%v out=%v", err, out)
	}
}

func TestApplyFileEditReplaceMatch(t *testing.T) {
	out, _, err := applyFileEdit([]string{"a", "port 80", "c"}, "replace_match", 0, "port", []string{"port 8080"})
	if err != nil || strings.Join(out, ",") != "a,port 8080,c" {
		t.Fatalf("err=%v out=%v", err, out)
	}
}

func TestApplyFileEditInsertAfterMatch(t *testing.T) {
	out, _, err := applyFileEdit([]string{"server {", "listen 80;", "}"}, "insert_after_match", 0, "server {", []string{"server_name x.com;"})
	if err != nil || strings.Join(out, "|") != "server {|server_name x.com;|listen 80;|}" {
		t.Fatalf("err=%v out=%v", err, out)
	}
}

func TestApplyFileEditOutOfRange(t *testing.T) {
	if _, _, err := applyFileEdit([]string{"a"}, "replace", 5, "", []string{"x"}); err == nil {
		t.Fatal("行号越界应报错")
	}
	if _, _, err := applyFileEdit([]string{"a"}, "nope", 1, "", nil); err == nil {
		t.Fatal("未知操作应报错")
	}
}

func TestApplyFileEditMatchNotFound(t *testing.T) {
	if _, _, err := applyFileEdit([]string{"a"}, "replace_match", 0, "zzz", []string{"x"}); err == nil {
		t.Fatal("关键字未找到应报错")
	}
}

func TestShellQuote(t *testing.T) {
	if shellQuote("/etc/nginx/nginx.conf") != "'/etc/nginx/nginx.conf'" {
		t.Fatal("普通路径错误")
	}
	if shellQuote("it's") != `'it'\''s'` {
		t.Fatal("单引号转义错误")
	}
}
