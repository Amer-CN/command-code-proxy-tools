package zcoderemote

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// pythonPath 探测测试可用的 Python 命令。
func pythonPath(t *testing.T) string {
	t.Helper()
	for _, c := range []string{"python", "py"} {
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	t.Skip("本机无 python，跳过子进程 mock 测试")
	return ""
}

// mockServerScript 返回一个 ndjson mock app-server 脚本：
//   - 每行读请求，按 id 原样回 envelope
//   - method=="echo"      → 回 {"id":N,"result":{...params 回显...}}
//   - method=="fail"      → 回 {"error":{"code":-32601,"message":...},"id":N}
//   - method=="slow"      → 睡 30s 再回（用于测超时）
//   - method=="die"       → 进程直接退出（用于测崩溃清理）
var mockServerScript = strings.ReplaceAll(`
import json, sys, time

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
    except Exception:
        continue
    rid = req.get("id")
    method = req.get("method", "")
    params = req.get("params") or {}
    if method == "echo":
        out = {"id": rid, "result": {"echo": params}}
    elif method == "fail":
        out = {"error": {"code": -32601, "message": params.get("message", "mock 失败")}, "id": rid}
    elif method == "slow":
        time.sleep(30)
        out = {"id": rid, "result": "late"}
    elif method == "die":
        sys.exit(3)
    else:
        out = {"error": {"code": -32601, "message": "method not found: " + method}, "id": rid}
    sys.stdout.write(json.dumps(out, separators=(",", ":")) + "\n")
    sys.stdout.flush()
`, "\r\n", "\n")

// startMockProc 用 python 子进程起一个 mock app-server（ndjson over stdio）。
func startMockProc(t *testing.T) *Proc {
	t.Helper()
	py := pythonPath(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "mock_appserver.py")
	if err := os.WriteFile(script, []byte(mockServerScript), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewProc(py, []string{script}, "")
	if err := p.Start(); err != nil {
		t.Fatalf("启动 mock app-server 失败: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// ---------- 协议编解码 / id 匹配 ----------

func TestCallEchoRoundtrip(t *testing.T) {
	p := startMockProc(t)
	res, err := p.Call("echo", map[string]any{"k": "v"}, 5*time.Second)
	if err != nil {
		t.Fatalf("Call 失败: %v", err)
	}
	var got struct {
		Echo map[string]any `json:"echo"`
	}
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("解析 result 失败: %v (%s)", err, res)
	}
	if got.Echo["k"] != "v" {
		t.Fatalf("回显不匹配: %v", got.Echo)
	}
}

func TestCallErrorEnvelope(t *testing.T) {
	p := startMockProc(t)
	_, err := p.Call("fail", map[string]any{"message": "nope"}, 5*time.Second)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "-32601") {
		t.Fatalf("错误信息应含 code 与 message: %v", err)
	}
}

// 并发 Call：id 各自匹配，不串线。
func TestCallConcurrentIDMatch(t *testing.T) {
	p := startMockProc(t)
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tag := fmt.Sprintf("req-%d", i)
			res, err := p.Call("echo", map[string]any{"tag": tag}, 10*time.Second)
			if err != nil {
				errs <- fmt.Errorf("req %d: %v", i, err)
				return
			}
			var got struct {
				Echo struct {
					Tag string `json:"tag"`
				} `json:"echo"`
			}
			if err := json.Unmarshal(res, &got); err != nil {
				errs <- fmt.Errorf("req %d 解析失败: %v", i, err)
				return
			}
			if got.Echo.Tag != tag {
				errs <- fmt.Errorf("req %d 串线: got %s", i, got.Echo.Tag)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// ---------- 超时 ----------

func TestCallTimeout(t *testing.T) {
	p := startMockProc(t)
	start := time.Now()
	_, err := p.Call("slow", nil, 500*time.Millisecond)
	if err == nil {
		t.Fatal("期望超时错误")
	}
	if !strings.Contains(err.Error(), "超时") && !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("错误应为超时: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("超时应立即返回，实际 %v", elapsed)
	}
	// 超时后连接仍可用：echo 立即回，早于 30s 的 slow 响应，可读出。
	if _, err := p.Call("echo", nil, 35*time.Second); err != nil {
		t.Fatalf("超时后连接应仍可用: %v", err)
	}
}

// ---------- 进程崩溃 ----------

func TestCallOnCrashedProcess(t *testing.T) {
	p := startMockProc(t)
	// 触发 mock 退出。
	if _, err := p.Call("die", nil, 5*time.Second); err == nil {
		t.Fatal("die 应返回错误（进程退出）")
	}
	// 崩溃后所有 pending Call 返回错误（此处验证崩溃后的新 Call 也报错）。
	if _, err := p.Call("echo", nil, 5*time.Second); err == nil {
		t.Fatal("崩溃后的 Call 应报错")
	}
}

// 并发请求挂起时进程崩溃 → 全部 pending 返回错误。
func TestCrashFailsAllPending(t *testing.T) {
	p := startMockProc(t)
	const n = 4
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := p.Call("slow", nil, 15*time.Second)
			errs <- err
		}()
	}
	// 等 slow 请求都进入 pending 后杀死进程。
	time.Sleep(300 * time.Millisecond)
	_ = p.cmd.Process.Kill()
	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			if err == nil {
				t.Fatal("pending Call 在进程崩溃后应返回错误")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("pending Call 未在崩溃后被唤醒")
		}
	}
}

// ---------- 未启动 / 关闭 ----------

func TestCallBeforeStart(t *testing.T) {
	p := NewProc("python", []string{"-c", "pass"}, "")
	if _, err := p.Call("echo", nil, time.Second); err == nil {
		t.Fatal("未启动的 Proc Call 应报错")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	p := startMockProc(t)
	if err := p.Close(); err != nil {
		t.Fatalf("第一次 Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("第二次 Close 应幂等: %v", err)
	}
}

// ---------- 路径探测 ----------

func TestDetectZCodeCJSNotFound(t *testing.T) {
	orig := zcodeCJSCandidates
	origProc := runningZCodePaths
	zcodeCJSCandidates = []string{filepath.Join(t.TempDir(), "不存在", "zcode.cjs")}
	runningZCodePaths = func() []string { return nil }
	defer func() {
		zcodeCJSCandidates = orig
		runningZCodePaths = origProc
	}()
	_, err := DetectZCodeCJS()
	if err == nil {
		t.Fatal("探测失败时应返回错误")
	}
	if !strings.Contains(err.Error(), "ZCode") {
		t.Fatalf("错误应提示安装 ZCode: %v", err)
	}
}

func TestDetectZCodeCJSFromRunning(t *testing.T) {
	// runningZCodePaths 返回的是 ZCode.exe 完整路径；cjs 在同级 resources/glm 下。
	exeDir := t.TempDir()
	fakeExe := filepath.Join(exeDir, "ZCode.exe")
	if err := os.WriteFile(fakeExe, []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	cjs := filepath.Join(exeDir, "resources", "glm", "zcode.cjs")
	if err := os.MkdirAll(filepath.Dir(cjs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cjs, []byte("// mock"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := zcodeCJSCandidates
	origProc := runningZCodePaths
	zcodeCJSCandidates = []string{filepath.Join(t.TempDir(), "nope", "zcode.cjs")}
	runningZCodePaths = func() []string { return []string{fakeExe} }
	defer func() {
		zcodeCJSCandidates = orig
		runningZCodePaths = origProc
	}()
	got, err := DetectZCodeCJS()
	if err != nil {
		t.Fatalf("应从运行进程路径推导: %v", err)
	}
	if got != cjs {
		t.Fatalf("推导结果不匹配: %s != %s", got, cjs)
	}
}
