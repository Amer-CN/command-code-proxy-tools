// baiproxy — B.AI 本地透明转发（兼容入口）。
//
// 收编自独立转发实现：核心逻辑已迁到 internal/bai 包（本仓库唯一的 b.ai 转发实现，
// 也供 GUI 的原生插件「B.AI」复用）。本入口仅为兼容 `baiproxy.exe` 旧调用方式保留：
// 与 GUI 托管同一转发服务，共用端口 8891，请勿与 GUI 插件同时运行。
//
// 背景：b.ai 是标准 OpenAI 兼容 API（可直接调用），但部分客户端/网络栈
// （如 Node/Electron 的 fetch）连 api.b.ai 时连接层会间歇失败（fetch failed），
// 而 Go 的 HTTP 栈实测最稳。本工具监听本地端口，把请求用 Go 栈转发给上游，
// ZCode 等客户端配 baseURL = http://127.0.0.1:8891/v1 即可使用。
//
// 用法：
//   baiproxy                       监听 127.0.0.1:8891 -> https://api.b.ai
//   baiproxy -addr 127.0.0.1:8891
package main

import (
	"flag"
	"log"
	"net"

	"github.com/dev2k6/command-code-proxy-server/internal/bai"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8891", "本地监听地址（host:port）")
	flag.Parse()

	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatalf("监听地址解析失败: %v", err)
	}
	if err := bai.NewServer().Start(host, port); err != nil {
		log.Fatalf("bai 转发启动失败: %v", err)
	}
}
