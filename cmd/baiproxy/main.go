// baiproxy — 本地透明转发：把任意 OpenAI 兼容请求转发到 api.b.ai。
//
// 背景：b.ai 是标准 OpenAI 兼容 API（可直接调用），但部分客户端/网络栈
// （如 Node/Electron 的 fetch）连 api.b.ai 时连接层会间歇失败（fetch failed），
// 而 Go 的 HTTP 栈实测最稳（8/8 成功）。本工具监听本地端口，把请求用 Go 栈
// 转发给上游，ZCode 等客户端配 baseURL = http://127.0.0.1:8891/v1 即可使用。
//
// 用法：
//   baiproxy                       监听 127.0.0.1:8891 -> https://api.b.ai
//   baiproxy -port 8891 -upstream https://api.b.ai
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8891", "本地监听地址")
	upstream := flag.String("upstream", "https://api.b.ai", "上游 base URL")
	flag.Parse()

	target, err := url.Parse(*upstream)
	if err != nil {
		log.Fatalf("上游地址解析失败: %v", err)
	}
	// 用自定义 Director：必须把 Host 改成上游（Cloudflare 对不认识的主机名直接 403），
	// 并保持其余请求头原样透传（Authorization 由客户端携带）。
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
	}
	proxy := &httputil.ReverseProxy{
		Director:      director,
		FlushInterval: 50 * time.Millisecond, // SSE 流式及时刷新
	}
	log.Printf("baiproxy 监听 %s → %s（OpenAI 兼容转发）", *addr, target)
	log.Fatal(http.ListenAndServe(*addr, proxy))
}
