//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	webview "github.com/webview/webview_go"
)

// app 是 GUI 与代理核心之间的控制器：代理直接运行在本进程内。
type app struct {
	host string
	port string

	mu       sync.Mutex
	httpd    *http.Server
	running  bool // 本进程内的代理正在运行
	external bool // 端口被本程序之外的代理实例占用（如开机自启的后台实例）
	started  time.Time
	apiKey   string
	lastErr  string
}

func newApp(host, port, key string) *app {
	a := &app{host: host, port: port, apiKey: key}
	if a.apiKey == "" {
		if b, err := os.ReadFile(a.keyFile()); err == nil {
			a.apiKey = strings.TrimSpace(string(b))
		}
	}
	return a
}

func (a *app) keyFile() string    { return filepath.Join(exeDir(), "api-key.txt") }
func (a *app) statsFile() string  { return filepath.Join(exeDir(), "stats.json") }
func (a *app) noticeFile() string { return filepath.Join(exeDir(), "notice_dismissed.flag") }
func (a *app) closeHintFile() string { return filepath.Join(exeDir(), "close_hint_dismissed.flag") }
func (a *app) baseURL() string    { return "http://" + net.JoinHostPort(a.host, a.port) }
func (a *app) healthURL() string  { return a.baseURL() + "/health" }

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// migrateLegacyStats 把旧版（bin\stats.json）的统计数据迁移到新位置，仅首次生效。
func (a *app) migrateLegacyStats() {
	dst := a.statsFile()
	if fileExists(dst) {
		return
	}
	src := filepath.Join(exeDir(), "bin", "stats.json")
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, in)
}

// start 启动代理。代理以 headless 子进程常驻（独立于本 GUI 进程），
// 因此关闭窗口不影响代理；再次打开 GUI 时探活识别为"运行中"。
func (a *app) start(key string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if key == "" {
		key = a.apiKey
	}
	if key != "" {
		a.apiKey = key
		_ = os.WriteFile(a.keyFile(), []byte(key), 0o600)
	}

	// 端口已有健康代理（可能是上次遗留的 headless 子进程）→ 直接接管观察。
	if httpOK(a.healthURL()) {
		a.running = true
		a.external = false
		a.started = time.Now()
		a.lastErr = ""
		return "检测到 " + a.baseURL() + " 代理已在运行，已接入", nil
	}

	// 端口被非健康进程占用（僵死/残留）→ 先清理，再启动。
	if portBusy(a.port) {
		if err := killByPort(a.port); err != nil {
			// 清理失败不阻塞：子进程绑定端口时若仍冲突会再报错
			_ = err
		}
		time.Sleep(300 * time.Millisecond)
	}

	// 启动 headless 子进程（同一 exe，-headless 参数），代理独立常驻。
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位自身可执行文件: %v", err)
	}
	cmd := exec.Command(exe, "-headless")
	if key != "" {
		cmd.Args = append(cmd.Args, "-api-key", key)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动后台代理失败: %v", err)
	}

	// 等待代理就绪（最多 ~5s）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if httpOK(a.healthURL()) {
			a.running = true
			a.external = false
			a.started = time.Now()
			a.lastErr = ""
			return "点火成功 · " + a.baseURL() + " 已就绪（后台常驻，关窗不影响）", nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", fmt.Errorf("后台代理启动超时，请检查端口 %s 是否被占用", a.port)
}

// stop 停堆：探活 → 若有代理在跑（无论是不是本进程起的 headless 子进程），
// 结束占用端口的进程。本 GUI 进程自身不持有代理，停堆只影响后台代理。
func (a *app) stop() (string, error) {
	a.mu.Lock()
	wasExternal := a.external
	a.mu.Unlock()

	if !httpOK(a.healthURL()) {
		a.mu.Lock()
		a.running = false
		a.external = false
		a.mu.Unlock()
		return "代理未在运行", nil
	}

	if err := killByPort(a.port); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.running = false
	a.external = false
	a.mu.Unlock()
	if wasExternal {
		return "后台代理已停止", nil
	}
	return "代理核心已停堆（后台进程已结束）", nil
}

// stateMsg 是发给界面的完整状态。
type stateMsg struct {
	Running         bool   `json:"running"`
	External        bool   `json:"external"`
	Phase           string `json:"phase"` // idle | running | external | error
	Host            string `json:"host"`
	Port            string `json:"port"`
	BaseURL         string `json:"baseUrl"`
	Uptime          int64  `json:"uptime"`
	Autostart       bool   `json:"autostart"`
	APIKey          string `json:"apiKey"`
	NoticeDismissed bool   `json:"noticeDismissed"`
	CloseHintDone    bool   `json:"closeHintDismissed"`
	Version         string `json:"version"`
	Core            string `json:"core"`
	PID             int    `json:"pid"`
	LastErr         string `json:"lastErr,omitempty"`
}

func (a *app) state() stateMsg {
	a.mu.Lock()
	running, external, started, lastErr, key := a.running, a.external, a.started, a.lastErr, a.apiKey
	a.mu.Unlock()

	// 探活：端口上有健康代理即视为运行中（可能是本 GUI 起的 headless 子进程，
	// 也可能是开机自启/上次遗留的后台实例）。GUI 关闭不影响代理运行。
	alive := httpOK(a.healthURL())
	if alive && !running {
		running = true
		a.mu.Lock()
		a.running = true
		if a.started.IsZero() {
			a.started = time.Now()
		}
		a.mu.Unlock()
		started = a.started
	}

	phase := "idle"
	switch {
	case running:
		phase = "running"
	case lastErr != "":
		phase = "error"
	}
	var up int64
	if running {
		up = int64(time.Since(started).Seconds())
	}
	return stateMsg{
		Running: running, External: external, Phase: phase,
		Host: a.host, Port: a.port, BaseURL: a.baseURL() + "/v1",
		Uptime: up, Autostart: autostartInstalled(), APIKey: key,
		NoticeDismissed: fileExists(a.noticeFile()),
		CloseHintDone:   fileExists(a.closeHintFile()),
		Version: appVersion, Core: coreVersion, PID: os.Getpid(), LastErr: lastErr,
	}
}

/* ---------------- 开机自启 ---------------- */

func autostartVBS() string {
	return filepath.Join(os.Getenv("APPDATA"),
		`Microsoft\Windows\Start Menu\Programs\Startup`, "command-code-proxy-autostart.vbs")
}

func autostartInstalled() bool { return fileExists(autostartVBS()) }

func setAutostart(on bool, port string) (string, error) {
	vbs := autostartVBS()
	if !on {
		if fileExists(vbs) {
			if err := os.Remove(vbs); err != nil {
				return "", err
			}
		}
		return "开机自启已取消", nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if rp, err := filepath.EvalSymlinks(exe); err == nil {
		exe = rp
	}
	// VBS 字符串内以两个双引号转义一个双引号
	content := "Set sh = CreateObject(\"WScript.Shell\")\r\n" +
		"sh.Run \"\"\"" + exe + "\"\" -headless -port " + port + "\", 0, False\r\n"
	if err := os.WriteFile(vbs, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("写入启动文件夹失败（被杀软拦截？）: %v", err)
	}
	return "开机自启已设置 · 登录 Windows 后将以无窗口模式自动点火", nil
}

/* ---------------- 工具函数 ---------------- */

var httpClient = &http.Client{Timeout: 1500 * time.Millisecond}

func httpGet(url string) (string, bool) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", false
	}
	return string(b), true
}

func httpOK(url string) bool { _, ok := httpGet(url); return ok }

// portBusy 检查端口是否有进程监听（不论是否健康）。
func portBusy(port string) bool {
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && strings.EqualFold(f[3], "LISTENING") && strings.HasSuffix(f[1], ":"+port) {
			return true
		}
	}
	return false
}

// killByPort 结束监听指定端口的进程（排除自己），用于停止外部/遗留实例。
func killByPort(port string) error {
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return fmt.Errorf("netstat 执行失败: %v", err)
	}
	self := os.Getpid()
	killed := false
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		// TCP  127.0.0.1:55990  0.0.0.0:0  LISTENING  12345
		if len(f) >= 5 && strings.EqualFold(f[3], "LISTENING") && strings.HasSuffix(f[1], ":"+port) {
			pid, _ := strconv.Atoi(f[4])
			if pid > 0 && pid != self {
				_ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
				killed = true
			}
		}
	}
	if !killed {
		return errors.New("未找到占用该端口的进程（可能已自行退出）")
	}
	return nil
}

func openURL(u string) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("仅允许打开 http(s) 链接")
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
}

/* ---------------- JS 桥接绑定 ---------------- */

func jsonOK(msg string) string {
	b, _ := json.Marshal(map[string]any{"ok": true, "msg": msg})
	return string(b)
}

func jsonErr(err error) string {
	b, _ := json.Marshal(map[string]any{"ok": false, "msg": err.Error()})
	return string(b)
}

func (a *app) bindAll(w webview.WebView) {
	_ = w.Bind("ccGetState", func() string {
		b, _ := json.Marshal(a.state())
		return string(b)
	})
	_ = w.Bind("ccStart", func(key string) string {
		msg, err := a.start(strings.TrimSpace(key))
		if err != nil {
			return jsonErr(err)
		}
		return jsonOK(msg)
	})
	_ = w.Bind("ccStop", func() string {
		msg, err := a.stop()
		if err != nil {
			return jsonErr(err)
		}
		return jsonOK(msg)
	})
	// 透传代理的统计数据（原始 JSON，前端直接解析 total/today/models）
	_ = w.Bind("ccStats", func() string {
		if s, ok := httpGet(a.baseURL() + "/v1/stats"); ok {
			return s
		}
		return `{"ok":false,"msg":"代理未运行或无法连接"}`
	})
	_ = w.Bind("ccCalib", func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			_ = os.Remove(filepath.Join(exeDir(), "calibration.txt"))
			return jsonOK("已清除校准")
		}
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return jsonErr(fmt.Errorf("请输入数字金额"))
		}
		if err := os.WriteFile(filepath.Join(exeDir(), "calibration.txt"), []byte(v), 0o600); err != nil {
			return jsonErr(err)
		}
		return jsonOK("校准已保存")
	})
	_ = w.Bind("ccModels", func() string {
		if s, ok := httpGet(a.baseURL() + "/v1/models"); ok {
			return s
		}
		return `{"ok":false,"msg":"代理未运行或无法连接"}`
	})
	_ = w.Bind("ccAutostart", func(on bool) string {
		msg, err := setAutostart(on, a.port)
		if err != nil {
			return jsonErr(err)
		}
		return jsonOK(msg)
	})
	_ = w.Bind("ccOpen", func(u string) string {
		if err := openURL(u); err != nil {
			return jsonErr(err)
		}
		return jsonOK("已打开")
	})
	_ = w.Bind("ccCopy", func(s string) string {
		if err := clipboard.WriteAll(s); err != nil {
			return jsonErr(err)
		}
		return jsonOK("已复制")
	})
	_ = w.Bind("ccDismiss", func() string {
		if err := os.WriteFile(a.noticeFile(), []byte("1"), 0o600); err != nil {
			return jsonErr(err)
		}
		return jsonOK("已记录")
	})
	_ = w.Bind("ccDismissCloseHint", func() string {
		if err := os.WriteFile(a.closeHintFile(), []byte("1"), 0o600); err != nil {
			return jsonErr(err)
		}
		return jsonOK("已记录")
	})
}
