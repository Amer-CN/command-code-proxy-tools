// Package zcoderemote 把多个 ZCode 账号（独立 USERPROFILE slot）的官方
// 赠送额度聚合成本地 OpenAI 兼容 API。原理：
//  1. node "…/resources/glm/zcode.cjs" app-server --stdio 是官方 ZCode
//     Protocol stdio 服务，帧格式 ndjson：请求 {id,method,params}，
//     响应 {id,result} / {id,error}，无握手。
//  2. 环境变量 USERPROFILE=<slot目录> 重定向后，app-server 在该目录下自建
//     全新 .zcode 结构，与其他账号完全隔离。
//  3. B 账号登录：GUI 用 USERPROFILE=<slot> 启动完整 ZCode 桌面端，
//     用户登录后登录态落盘到 slot 目录，之后无头 app-server 即可复用。
package zcoderemote

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// 默认请求超时（generateText 长生成可能很慢，给足）。
const defaultCallTimeout = 120 * time.Second

// zcodeCJSCandidates 是 zcode.cjs 的候选路径（探测顺序即优先级）。
var zcodeCJSCandidates = []string{
	`D:\Program Files\ZCode\resources\glm\zcode.cjs`,
}

// runningZCodePaths 返回正在运行的 ZCode.exe 可执行文件路径（用 tasklist 探测）。
// 变量化便于测试注入。
var runningZCodePaths = func() []string {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq ZCode.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}
	// CSV 输出不含可执行路径（tasklist 无 Path 列），改用 wmic 查 ExecutablePath。
	// wmic 在新系统可能缺失，失败则返回 nil。
	out2, err := exec.Command("wmic", "process", "where", "name='ZCode.exe'", "get", "ExecutablePath", "/VALUE").Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, ln := range splitLines(string(out2)) {
		ln = trimSpace(ln)
		const pfx = "ExecutablePath="
		if len(ln) > len(pfx) && ln[:len(pfx)] == pfx {
			p := ln[len(pfx):]
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	_ = out
	return paths
}

// DetectZCodeCJS 探测官方 zcode.cjs 路径：
// 优先写死路径，失败则遍历运行中的 ZCode.exe 路径推导（…/resources/glm/zcode.cjs）。
func DetectZCodeCJS() (string, error) {
	for _, p := range zcodeCJSCandidates {
		if fileExists(p) {
			return p, nil
		}
	}
	for _, exe := range runningZCodePaths() {
		cjs := filepath.Join(filepath.Dir(exe), "resources", "glm", "zcode.cjs")
		if fileExists(cjs) {
			log.Printf("[zcoderemote] 从运行中的 ZCode 进程推导 zcode.cjs: %s", cjs)
			return cjs, nil
		}
	}
	return "", fmt.Errorf("未找到 ZCode（探测 %v 与运行中的 ZCode.exe 均失败）。请先安装 ZCode 桌面端", zcodeCJSCandidates)
}

// DetectNode 探测可用的 node 命令。
func DetectNode() (string, error) {
	if p, err := exec.LookPath("node"); err == nil {
		return p, nil
	}
	return "", errors.New("未找到 node 命令。ZCode Protocol 需要 Node.js 运行时（随 ZCode 桌面端安装，或自行安装 Node.js 并加入 PATH）")
}

// Proc 封装一个 app-server 子进程（ndjson over stdio，线程安全）。
type Proc struct {
	// 命令与环境（Start 前设置）
	nodeBin string   // node 可执行文件
	nodeArg []string // node 参数（zcode.cjs app-server --stdio）
	envDir  string   // 非空 = USERPROFILE 重定向到该目录（slot 隔离）

	cmd   *exec.Cmd
	stdin io.WriteCloser // app-server 的 stdin（Start 时接管）

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan *wireMsg
	closed  bool
}

// wireMsg 是 ndjson envelope 的统一表示。
type wireMsg struct {
	ID     *int64          `json:"id"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *wireError      `json:"error,omitempty"`
}

// wireError 是错误 envelope 里的 error 对象。
type wireError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *wireError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("zcode 错误 %d: %s (%s)", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("zcode 错误 %d: %s", e.Code, e.Message)
}

// NewProc 创建一个 app-server 进程封装。envDir 非空时 USERPROFILE 重定向。
func NewProc(nodeBin string, nodeArg []string, envDir string) *Proc {
	return &Proc{
		nodeBin: nodeBin,
		nodeArg: nodeArg,
		envDir:  envDir,
		pending: map[int64]chan *wireMsg{},
	}
}

// NewSlotProc 为指定 slot 目录创建 app-server 进程封装（懒启动）。
// zcode.cjs / node 路径在此刻探测（提前暴露环境问题）。
func NewSlotProc(envDir string) (*Proc, error) {
	cjs, err := DetectZCodeCJS()
	if err != nil {
		return nil, err
	}
	node, err := DetectNode()
	if err != nil {
		return nil, err
	}
	return NewProc(node, []string{cjs, "app-server", "--stdio"}, envDir), nil
}

// Start 启动子进程并进入读循环。
func (p *Proc) Start() error {
	p.mu.Lock()
	if p.cmd != nil {
		p.mu.Unlock()
		return errors.New("app-server 进程已启动")
	}
	p.mu.Unlock()

	cmd := exec.Command(p.nodeBin, p.nodeArg...)
	cmd.Env = os.Environ()
	if p.envDir != "" {
		// ZCODE_DATA_BASE_DIR：官方环境变量，credentials 路径优先读它
		// （zcode.cjs: baseDir ?? env.ZCODE_DATA_BASE_DIR ?? homedir()）。
		// 不传 USERPROFILE：凭据加密 key fallback 派生自 platform:homedir:username，
		// 改 homedir 会导致登录态解密失败。
		cmd.Env = append(cmd.Env, "ZCODE_DATA_BASE_DIR="+p.envDir)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("接管 stdin 失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("接管 stdout 失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 app-server 失败: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	p.stdin = stdin
	p.closed = false
	p.mu.Unlock()

	go p.readLoop(stdout)
	// 进程退出（正常/崩溃/被杀）→ 唤醒所有 pending 并关 stdin。
	go func() {
		_ = cmd.Wait()
		p.failAll(fmt.Errorf("app-server 进程已退出"))
		p.mu.Lock()
		w := p.stdin
		p.stdin = nil
		p.mu.Unlock()
		if w != nil {
			_ = w.Close()
		}
	}()
	return nil
}

// readLoop 逐行读 ndjson 响应并按 id 分发给 pending。
func (p *Proc) readLoop(r io.Reader) {
	sc := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := sc.ReadBytes('\n')
		if len(line) > 0 {
			var msg wireMsg
			if json.Unmarshal(trimJSONWhitespace(line), &msg) == nil && msg.ID != nil {
				ch := p.takePending(*msg.ID)
				if ch != nil {
					ch <- &msg
				}
				continue
			}
			// 非 envelope 行（日志等）：忽略
			log.Printf("[zcoderemote] app-server 输出: %s", truncateStr(string(line), 200))
		}
		if err != nil {
			p.failAll(fmt.Errorf("app-server 输出流结束: %w", err))
			return
		}
	}
}

// takePending 取出并删除一个 pending 通道（响应到达时调用）。
func (p *Proc) takePending(id int64) chan *wireMsg {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.pending[id]
	if ok {
		delete(p.pending, id)
	}
	return ch
}

// failAll 让所有 pending Call 立即失败（进程崩溃/输出流结束）。
func (p *Proc) failAll(err error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	pending := p.pending
	p.pending = map[int64]chan *wireMsg{}
	p.mu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- &wireMsg{Error: &wireError{Code: -1, Message: err.Error()}}:
		default: // 通道缓冲 1，不会阻塞；保险起见
		}
	}
}

// Call 发送一个请求并等待响应（带超时）。超时只放弃该请求，不杀进程。
func (p *Proc) Call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = defaultCallTimeout
	}
	p.mu.Lock()
	if p.cmd == nil {
		p.mu.Unlock()
		return nil, errors.New("app-server 未启动")
	}
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("app-server 进程已退出")
	}
	p.nextID++
	id := p.nextID
	ch := make(chan *wireMsg, 1)
	p.pending[id] = ch
	stdin := p.stdin
	p.mu.Unlock()

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			p.takePending(id)
			return nil, fmt.Errorf("序列化 params 失败: %w", err)
		}
		rawParams = b
	}
	req, err := json.Marshal(wireMsg{ID: &id, Method: method, Params: rawParams})
	if err != nil {
		p.takePending(id)
		return nil, err
	}
	if stdin == nil {
		p.takePending(id)
		return nil, errors.New("app-server stdin 不可用")
	}

	// 写请求（管道写串行锁保护；带超时防止管道堵塞挂死）。
	werr := make(chan error, 1)
	go func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		_, err := stdin.Write(append(req, '\n'))
		werr <- err
	}()
	select {
	case err := <-werr:
		if err != nil {
			p.takePending(id)
			return nil, fmt.Errorf("发送请求失败: %w", err)
		}
	case <-time.After(5 * time.Second):
		p.takePending(id)
		return nil, errors.New("发送请求超时（stdin 堵塞）")
	}

	// 等响应 / 超时。
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, msg.Error
		}
		return msg.Result, nil
	case <-timer.C:
		p.takePending(id)
		return nil, fmt.Errorf("zcode 请求 %s 超时（%s）", method, timeout)
	}
}

// Alive 报告进程是否存活（已启动且未崩溃）。
func (p *Proc) Alive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cmd != nil && !p.closed
}

// Close 优雅关闭：杀进程并唤醒所有 pending（幂等）。
func (p *Proc) Close() error {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	p.failAll(errors.New("app-server 已关闭"))
	return nil
}

// ---------- 工具 ----------

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// trimJSONWhitespace 去掉行尾 \r\n 等空白（ndjson 行转 JSON）。
func trimJSONWhitespace(b []byte) []byte {
	for len(b) > 0 {
		c := b[len(b)-1]
		if c == '\n' || c == '\r' || c == ' ' || c == '\t' {
			b = b[:len(b)-1]
		} else {
			break
		}
	}
	return b
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// splitLines 按通用换行符切行（wmic 输出是 \r\n）。
func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

// itoaA 小工具（避免多处 strconv 散落）。
func itoaA(n int) string { return strconv.Itoa(n) }
