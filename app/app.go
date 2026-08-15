//go:build !private

// app.go —— 公开版 app 结构（无插件托管字段）。
// 完整版结构见作者本地 .private/app/app.go（构建时注入，含插件状态）。
package main

import (
	"net/http"
	"sync"
	"time"
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
