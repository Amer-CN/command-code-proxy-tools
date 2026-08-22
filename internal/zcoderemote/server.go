// server.go —— 本地 OpenAI 兼容端点（默认 8792）：
//
//	/v1/models              固定模型列表（B 账号赠送额度可用模型）
//	/v1/chat/completions    messages → 选账号 slot → generateText → OpenAI 格式
//	/health                 账号数/启用数/进程状态；ZCode 缺失时返回明确错误
//	/accounts 系列          GUI「多开」视图后端 API
package zcoderemote

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 固定模型列表（B 账号赠送额度可用模型，硬编码）。
var fixedModels = []string{"GLM-5.3", "GLM-5.2", "GLM-5-Turbo"}

// Server 是 ZCode 多账号聚合的本地 OpenAI 兼容服务。
type Server struct {
	accounts *Accounts

	ln  net.Listener
	mu  sync.Mutex
	srv *http.Server

	startedAt time.Time
}

// NewServer 创建服务（slot 根目录 = exe 同目录/zcode-accounts）。
func NewServer() *Server {
	return NewServerWithRoot(DefaultAccountsRoot())
}

// NewServerWithRoot 创建服务并指定 slot 根目录（测试用）。
func NewServerWithRoot(rootDir string) *Server {
	return &Server{
		accounts:  NewAccounts(rootDir),
		startedAt: time.Now(),
	}
}

// corsWith 给所有响应加 CORS 头：GUI 页面跑在 localhost:随机端口，
// fetch 127.0.0.1:8792 属跨域，没有这个头前端全部拉不到数据。
func corsWith(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// Start 在 host:port 上监听（阻塞）。
func (s *Server) Start(host, port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChat)
	mux.HandleFunc("/accounts", s.handleAccounts)
	mux.HandleFunc("/accounts/", s.handleAccountItem)

	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("端口 %s 被占用: %w", port, err)
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	srv := &http.Server{Handler: corsWith(mux)}
	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()
	log.Printf("[zcoderemote] 服务已启动 %s:%s（账号目录 %s）", host, port, s.accounts.rootDir)
	return srv.Serve(ln)
}

// Stop 停止服务并清理全部 app-server。
func (s *Server) Stop() {
	s.accounts.CloseAll()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		_ = s.srv.Close()
	}
}

// ---------- /health ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"status": "ok", "service": "zcoderemote-go"}
	if !s.startedAt.IsZero() {
		resp["uptimeSec"] = int64(time.Since(s.startedAt).Seconds())
	}
	// ZCode 环境探测失败 → 明确报错（无 ZCode 环境降级提示）。
	if _, err := DetectZCodeCJS(); err != nil {
		resp["status"] = "error"
		resp["message"] = err.Error()
		writeJSON(w, resp)
		return
	}
	if _, err := DetectNode(); err != nil {
		resp["status"] = "error"
		resp["message"] = err.Error()
		writeJSON(w, resp)
		return
	}
	accounts := s.accounts.List()
	enabled, loggedIn := 0, 0
	for _, ac := range accounts {
		if ac.Enabled {
			enabled++
		}
		if ac.HasLogin {
			loggedIn++
		}
	}
	resp["accounts"] = len(accounts)
	resp["enabled"] = enabled
	resp["loggedIn"] = loggedIn
	resp["procs"] = s.accounts.ProcStates()
	writeJSON(w, resp)
}

// ---------- /v1/models ----------

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	data := make([]map[string]any, 0, len(fixedModels))
	for _, m := range fixedModels {
		data = append(data, map[string]any{"id": m, "object": "model", "owned_by": "zcode"})
	}
	writeJSON(w, map[string]any{"object": "list", "data": data})
}

// ---------- /v1/chat/completions ----------

// chatRequest 是入站 OpenAI 对话请求（只取转发需要的字段）。
type chatRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

// generateTextResult 是 workspace/generateText 响应 result 的解析结构
// （params/result 真实 schema 未最终确认：result 可能是 {content|text|…}，
// 此处多字段兼容，联调时按实测收紧）。
type generateTextResult struct {
	Content string `json:"content"`
	Text    string `json:"text"`
	Message string `json:"message"`
	Output  string `json:"output"`
}

func (g *generateTextResult) text() string {
	if g.Content != "" {
		return g.Content
	}
	if g.Text != "" {
		return g.Text
	}
	if g.Message != "" {
		return g.Message
	}
	return g.Output
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil || len(req.Messages) == 0 {
		writeErr(w, 400, "请求体需为 OpenAI chat/completions 格式（含 messages）")
		return
	}
	model := req.Model
	if model == "" {
		model = fixedModels[0]
	}

	// 选账号（enabled 且 hasLogin，最低使用优先）。
	ac, err := s.accounts.PickAccount()
	if err != nil {
		writeErr(w, 503, err.Error())
		return
	}
	proc, err := s.accounts.ProcFor(ac.Slot)
	if err != nil {
		log.Printf("[zcoderemote] chat model=%s slot=%d 启动 app-server 失败: %v", model, ac.Slot, err)
		writeErr(w, 502, err.Error())
		return
	}

	// messages 的 content 可能是 string 或多段数组，统一压成纯文本。
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]any{"role": m.Role, "content": flattenContent(m.Content)})
	}
	// generateText params 结构未最终确认：按逆向参考构造，真实联调由主智能体做。
	params := map[string]any{"messages": msgs}
	if model != "" {
		params["model"] = model
	}

	res, err := proc.Call("workspace/generateText", params, 0)
	if err != nil {
		log.Printf("[zcoderemote] chat model=%s slot=%d generateText 失败: %v", model, ac.Slot, err)
		writeErr(w, 502, "generateText 失败: "+err.Error())
		return
	}
	var gr generateTextResult
	_ = json.Unmarshal(res, &gr)
	content := gr.text()
	if content == "" {
		// result 不是对象（可能是纯文本字符串）。
		var s2 string
		if json.Unmarshal(res, &s2) == nil && s2 != "" {
			content = s2
		}
	}
	if content == "" {
		content = "" // 空内容也按正常结构返回（schema 联调阶段便于排查）
	}
	log.Printf("[zcoderemote] chat model=%s slot=%d status=200 dur=%s len=%d",
		model, ac.Slot, time.Since(start).Round(time.Millisecond), len(content))

	now := time.Now().Unix()
	id := "chatcmpl-zcode-" + strconv.FormatInt(now, 10)
	choice := map[string]any{
		"index":         0,
		"message":       map[string]any{"role": "assistant", "content": content},
		"finish_reason": "stop",
	}
	if req.Stream {
		// ZCode Protocol 是请求-响应式（无 SSE）：非流式调用后组装成单块 SSE 返回。
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": now,
			"model":   model,
			"choices": []map[string]any{{
				"index":         0,
				"delta":         map[string]any{"role": "assistant", "content": content},
				"finish_reason": nil,
			}},
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", b)
		done := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": now,
			"model":   model,
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		}
		bd, _ := json.Marshal(done)
		fmt.Fprintf(w, "data: %s\n\n", bd)
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	writeJSON(w, map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": now,
		"model":   model,
		"choices": []map[string]any{choice},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	})
}

// flattenContent 把 OpenAI content（string 或 [{type,text}] 多段）压成纯文本。
func flattenContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var sb strings.Builder
		for _, p := range parts {
			sb.WriteString(p.Text)
		}
		return sb.String()
	}
	return string(raw)
}

// ---------- /accounts 系列（GUI「多开」视图后端） ----------

// handleAccounts 处理 GET /accounts（列表）、POST /accounts（新建）。
func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"accounts": s.accounts.List()})
	case http.MethodPost:
		ac, err := s.accounts.Create()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "account": ac})
	default:
		writeErr(w, 405, "method not allowed")
	}
}

// handleAccountItem 处理 POST /accounts/<n>/login-instance|save|toggle、DELETE /accounts/<n>。
func (s *Server) handleAccountItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/accounts/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, 404, "not found")
		return
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		writeErr(w, 400, "账号编号需为数字")
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case r.Method == http.MethodDelete && action == "":
		if err := s.accounts.Delete(n); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case r.Method == http.MethodPost && action == "login-instance":
		if err := s.accounts.LaunchLoginInstance(n); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "msg": "已启动独立 ZCode，请在其中登录目标账号"})
	case r.Method == http.MethodPost && action == "save":
		ac, err := s.accounts.SaveLogin(n)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "account": ac})
	case r.Method == http.MethodPost && action == "toggle":
		ac, err := s.accounts.Toggle(n)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "account": ac})
	default:
		writeErr(w, 404, "not found")
	}
}

// ---------- 通用 ----------

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"message": msg, "type": "api_error"},
	})
}
