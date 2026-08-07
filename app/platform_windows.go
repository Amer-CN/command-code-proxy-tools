//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// setDPIAware 让 WebView2 窗口在高 DPI 屏幕上清晰渲染（尽力而为）。
func setDPIAware() {
	defer func() { _ = recover() }()
	user32 := syscall.NewLazyDLL("user32.dll")
	// SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4)
	if p := user32.NewProc("SetProcessDpiAwarenessContext"); p != nil {
		if r, _, _ := p.Call(^uintptr(3)); r != 0 {
			return
		}
	}
	// 旧系统回退：SetProcessDPIAware()
	if p := user32.NewProc("SetProcessDPIAware"); p != nil {
		p.Call()
	}
}

// initialWindowSize 按主屏逻辑分辨率自适应初始窗口大小：
// 高分辨率屏（2K/4K）自动放大，普通 1080p 保持 1280x800，且不超出屏幕工作区。
func initialWindowSize() (int, int) {
	user32 := syscall.NewLazyDLL("user32.dll")
	sm := user32.NewProc("GetSystemMetrics")
	sw, _, _ := sm.Call(0) // SM_CXSCREEN
	sh, _, _ := sm.Call(1) // SM_CYSCREEN
	if sw <= 0 || sh <= 0 {
		return 1280, 800
	}
	w := int(sw) * 4 / 5
	h := int(sh) * 4 / 5
	if w > 1600 {
		w = 1600
	}
	if h > 1000 {
		h = 1000
	}
	if w < 1080 {
		w = 1080
	}
	if h < 680 {
		h = 680
	}
	// 兜底：任何情况下不超出屏幕
	if w > int(sw)*95/100 {
		w = int(sw) * 95 / 100
	}
	if h > int(sh)*95/100 {
		h = int(sh) * 95 / 100
	}
	return w, h
}

// enableDarkTitleBar 把原生标题栏切换为深色沉浸式（Win10 1809+ / Win11）。
func enableDarkTitleBar(hwnd uintptr) {
	defer func() { _ = recover() }()
	const DWMWA_USE_IMMERSIVE_DARK_MODE = 20
	p := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	var dark int32 = 1
	p.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&dark)), 4)
}

// fatalBox 弹出原生消息框（用于 WebView2 运行时缺失等启动期致命错误）。
func fatalBox(msg string) {
	defer func() { _ = recover() }()
	p := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(msg)
	c, _ := syscall.UTF16PtrFromString(appTitle)
	const MB_OK_ICONWARNING = 0x30
	p.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), MB_OK_ICONWARNING)
}
