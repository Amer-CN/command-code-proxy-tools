//go:build windows

package main

import (
	"context"
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
	"github.com/dev2k6/command-code-proxy-server/internal/proxy"
	"github.com/dev2k6/command-code-proxy-server/internal/server"
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

// start 进程内点火。返回 (提示语, error)；若端口上已有健康的外部实例，
// 返回 ("EXTERNAL:...", nil) 并进入 external 观察态。
func (a *app) start(key string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return "代理核心已在运行", nil
	}
	if key == "" {
		key = a.apiKey
	}
	// 端口已被健康代理占用 → 直接接管观察（通常是开机自启的后台实例）
	if httpOK(a.healthURL()) {
		a.external = true
		return "检测到 " + a.baseURL() + " 已有代理实例在运行（可能来自开机自启），已接入监测", nil
	}

	a.migrateLegacyStats()
	p := proxy.NewProxy(key)
	p.SetStatsFile(a.statsFile())
	srv := server.NewServer(p)

	ln, err := net.Listen("tcp", net.JoinHostPort(a.host, a.port))
	if err != nil {
		return "", fmt.Errorf("端口 %s 被其他程序占用，无法点火（可换个目录或端口重试）", a.port)
	}
	a.httpd = &http.Server{Handler: srv.Handler, ReadHeaderTimeout: 30 * time.Second}
	go func() {
		if e := a.httpd.Serve(ln); e != nil && !errors.Is(e, http.ErrServerClosed) {
			a.mu.Lock()
			a.lastErr = e.Error()
			a.running = false
			a.mu.Unlock()
		}
	}()
	a.running = true
	a.external = false
	a.started = time.Now()
	a.lastErr = ""
	if key != "" {
		a.apiKey = key
		_ = os.WriteFile(a.keyFile(), []byte(key), 0o600)
	}
	return "点火成功 · " + a.baseURL() + " 已就绪", nil
}

// stop 停堆；external 态下改为结束占用端口的外部进程。
func (a *app) stop() (string, error) {
	a.mu.Lock()
	if a.external && !a.running {
		a.mu.Unlock()
		if err := killByPort(a.port); err != nil {
			return "", err
		}
		a.mu.Lock()
		a.external = false
		a.mu.Unlock()
		return "外部代理实例已停止", nil
	}
	if !a.running {
		a.mu.Unlock()
		return "代理未在运行", nil
	}
	httpd := a.httpd
	a.httpd = nil
	a.running = false
	a.external = false
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpd.Shutdown(ctx)
	return "代理核心已停堆", nil
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
	Version         string `json:"version"`
	Core            string `json:"core"`
	PID             int    `json:"pid"`
	LastErr         string `json:"lastErr,omitempty"`
}

func (a *app) state() stateMsg {
	a.mu.Lock()
	running, external, started, lastErr, key := a.running, a.external, a.started, a.lastErr, a.apiKey
	a.mu.Unlock()

	phase := "idle"
	switch {
	case running:
		phase = "running"
	case external:
		phase = "external"
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
}
