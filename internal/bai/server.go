package bai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// 上游 base URL：api.b.ai 是标准 OpenAI 兼容端点。
// 注意：部分客户端/网络栈（Node/Electron fetch）连 api.b.ai 时连接层会间歇失败，
// 而 Go 的 HTTP 栈实测最稳。本服务监听本地端口把请求用 Go 栈转发给上游。
const upstreamBase = "https://api.b.ai"

// Server 是 B.AI 的本地 OpenAI 兼容转发服务。
type Server struct {
	ln        net.Listener
	srv       *http.Server
	startedAt time.Time
}

// NewServer 创建服务。
func NewServer() *Server {
	return &Server{startedAt: time.Now()}
}

// corsWith 给所有响应加 CORS 头：GUI 页面跑在 localhost:随机端口，
// fetch 127.0.0.1:8891 属跨域，没有这个头前端全部拉不到数据。
func corsWith(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// handleHealth 直接回 200（不转发上游，否则无 key 会 401 导致健康误判）。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"service":   "bai-go",
		"uptimeSec": int64(time.Since(s.startedAt).Seconds()),
	})
}

// Start 在 host:port 上监听（阻塞）。
// /v1/models 及一切其余路径 ReverseProxy 到 https://api.b.ai。
// Director 必须把 req.Host 改为 api.b.ai（Cloudflare 对不认识的主机名直接 403）；
// FlushInterval 50ms 保证 SSE 流式及时刷新。
func (s *Server) Start(host, port string) error {
	target, err := url.Parse(upstreamBase)
	if err != nil {
		return fmt.Errorf("上游地址解析失败: %v", err)
	}
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host // 必须改：Cloudflare 对未知 Host 直接 403
	}
	// api.b.ai 对某些网络（尤其国内直连）是间歇性失败的：连接层错误 / Cloudflare 520。
	// 520/502/503 意味着请求没到模型层（无计费），重放安全。
	// 前置 bufferBody 把请求体缓存为可重放（GetBody），POST 也能自动重试。
	retry := &retryRoundTripper{transport: http.DefaultTransport, max: 3}
	proxy := &httputil.ReverseProxy{
		Director:      director,
		Transport:     retry,
		FlushInterval: 50 * time.Millisecond, // SSE 流式及时刷新
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.Handle("/v1/models", proxy)
	mux.Handle("/v1/", proxy)
	mux.Handle("/", proxy)

	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("端口 %s 被占用: %w", port, err)
	}
	s.ln = ln
	s.srv = &http.Server{Handler: corsWith(bufferBody(mux))}
	return s.srv.Serve(ln)
}

// bufferBody 把带请求体的请求读入内存并设 GetBody（可重放），重试的前提。
// 聊天请求都是小 JSON（< 数 MB），缓冲代价可忽略；超限（>32MB）或读取失败的不缓冲（单发）。
func bufferBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		const limit = 32 << 20
		buf, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(buf))
		if err == nil && len(buf) <= limit {
			r.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(buf)), nil
			}
		}
		next.ServeHTTP(w, r)
	})
}

// retryRoundTripper 对可重放的请求做有限次重试：传输错误 / 5xx（含 Cloudflare 520）。
// 只在请求体可重放（GetBody 非空或无请求体）时重试；4xx 不重试（真实 API 应答）。
type retryRoundTripper struct {
	transport http.RoundTripper
	max       int
}

func (r *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.GetBody == nil {
		return r.transport.RoundTrip(req) // 不可重放：单发
	}
	var lastErr error
	for i := 0; i < r.max; i++ {
		last := i == r.max-1
		if i > 0 && req.GetBody != nil {
			if nb, err := req.GetBody(); err == nil {
				req.Body = nb
			}
		}
		resp, err := r.transport.RoundTrip(req)
		if err != nil {
			lastErr = err
			if last {
				return nil, err
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if resp.StatusCode < 500 || last {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8192)) // 5xx：丢弃响应体后重试
		_ = resp.Body.Close()
		time.Sleep(300 * time.Millisecond)
	}
	return nil, lastErr
}

// Stop 停止服务。
func (s *Server) Stop() {
	if s.srv != nil {
		_ = s.srv.Close()
	}
}