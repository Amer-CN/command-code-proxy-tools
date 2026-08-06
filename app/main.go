//go:build windows

// CommandCode Proxy Deck —— WebView2 独立 GUI 程序
//
// 与旧的 HTA 方案的区别：
//   - 代理核心直接在本进程内运行（internal/proxy + internal/server 的 Handler），
//     不再需要释放/调用外部 exe，点火即启动、关窗即优雅停堆。
//   - 界面由系统自带的 WebView2 运行时渲染（Win10/11 通常已预装），
//     编译产物是单个 exe，无控制台窗口（-H windowsgui），无需附带任何 DLL。
//   - 界面文件 ui.html 通过 go:embed 内嵌，并经 127.0.0.1 随机端口的进程内
//     HTTP 服务提供给 WebView2（http://localhost 属于安全上下文，剪贴板等 API 可用）。
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	webview "github.com/webview/webview_go"
)

const (
	appVersion  = "v2.0.0"
	coreVersion = "v1.0.8"
	appTitle    = "CommandCode 代理控制台"
)

//go:embed ui.html
var uiHTML string

var (
	flagPort     = flag.String("port", "55990", "代理监听端口")
	flagHost     = flag.String("host", "127.0.0.1", "代理监听地址")
	flagAPIKey   = flag.String("api-key", "", "CommandCode API Key（可选，也可在界面里填）")
	flagHeadless = flag.Bool("headless", false, "无窗口后台模式（供开机自启使用）")
	flagVersion  = flag.Bool("version", false, "打印版本并退出")
)

// exeDir 返回 exe 所在目录（go run 时退回工作目录），数据文件都放在这里。
func exeDir() string {
	if p, err := os.Executable(); err == nil {
		if rp, err := filepath.EvalSymlinks(p); err == nil {
			p = rp
		}
		return filepath.Dir(p)
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func main() {
	setDPIAware()
	flag.Parse()
	if *flagVersion {
		fmt.Printf("CommandCode Proxy Deck %s (proxy core %s)\n", appVersion, coreVersion)
		return
	}

	app := newApp(*flagHost, *flagPort, *flagAPIKey)

	// 无窗口后台模式：进程内启动代理后永久阻塞（由 taskkill / 界面停止）
	if *flagHeadless {
		if _, err := app.start(*flagAPIKey); err != nil {
			_ = os.WriteFile(filepath.Join(exeDir(), "headless-error.log"),
				[]byte(err.Error()), 0o600)
			os.Exit(1)
		}
		select {}
	}

	// GUI 模式：进程内静态服务提供内嵌界面（随机空闲端口，避免冲突）
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = fmt.Fprint(w, uiHTML)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatalBox("无法创建本地界面服务：\r\n" + err.Error())
		os.Exit(1)
	}
	go func() { _ = http.Serve(ln, mux) }()

	w := webview.New(false)
	if w == nil {
		fatalBox("未检测到 WebView2 运行时（Microsoft Edge WebView2 Runtime）。\r\n\r\nWindows 10/11 通常已随系统或 Edge 预装；若被卸载，请点「确定」打开下载页安装后重试。")
		_ = openURL("https://developer.microsoft.com/zh-cn/microsoft-edge/webview2/")
		os.Exit(1)
	}
	w.SetTitle(appTitle)
	w.SetSize(1280, 800, webview.HintNone)
	defer w.Destroy()
	defer func() { _, _ = app.stop() }() // 关窗时优雅停堆

	// 深色标题栏（Win10 1809+，尽力而为，失败无碍）
	if hwnd := w.Window(); hwnd != nil {
		enableDarkTitleBar(uintptr(hwnd))
	}
	w.Dispatch(func() {
		if hwnd := w.Window(); hwnd != nil {
			enableDarkTitleBar(uintptr(hwnd))
		}
	})
	w.SetSize(1080, 680, webview.HintMin) // 最小尺寸，防止布局被压坏

	app.bindAll(w)
	w.Navigate("http://" + ln.Addr().String() + "/")
	w.Run()
}
