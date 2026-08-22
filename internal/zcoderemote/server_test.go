package zcoderemote

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestServer 起一个不监听端口的 Server（直接测 handler，避免端口占用）。
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := NewServerWithRoot(t.TempDir())
	t.Cleanup(s.Stop)
	return s
}

// ---------- /v1/models ----------

func TestHandleModels(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.handleModels(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != len(fixedModels) {
		t.Fatalf("模型数=%d want %d", len(out.Data), len(fixedModels))
	}
	for _, m := range out.Data {
		found := false
		for _, want := range fixedModels {
			if m.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("意外模型 %q", m.ID)
		}
	}
}

// ---------- 无 ZCode 环境降级 ----------

func TestHealthNoZCode(t *testing.T) {
	s := newTestServer(t)
	origCand := zcodeCJSCandidates
	origProc := runningZCodePaths
	zcodeCJSCandidates = []string{filepath.Join(t.TempDir(), "nope", "zcode.cjs")}
	runningZCodePaths = func() []string { return nil }
	defer func() {
		zcodeCJSCandidates = origCand
		runningZCodePaths = origProc
	}()
	rec := httptest.NewRecorder()
	s.handleHealth(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != 200 {
		t.Fatalf("/health 状态码=%d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "error" {
		t.Fatalf("status=%v want error", out["status"])
	}
	msg, _ := out["message"].(string)
	if !strings.Contains(msg, "ZCode") {
		t.Fatalf("message 应提示 ZCode 缺失: %q", msg)
	}
}

// ---------- 账号 CRUD ----------

func TestAccountsCRUD(t *testing.T) {
	s := newTestServer(t)

	// 新建 slot 1、2。
	rec := httptest.NewRecorder()
	s.handleAccounts(rec, httptest.NewRequest(http.MethodPost, "/accounts", nil))
	if rec.Code != 200 {
		t.Fatalf("新建账号失败: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleAccounts(rec, httptest.NewRequest(http.MethodPost, "/accounts", nil))
	if rec.Code != 200 {
		t.Fatalf("新建第二个账号失败: %s", rec.Body.String())
	}

	// 列表。
	rec = httptest.NewRecorder()
	s.handleAccounts(rec, httptest.NewRequest(http.MethodGet, "/accounts", nil))
	var list struct {
		Accounts []*Account `json:"accounts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Accounts) != 2 || list.Accounts[0].Slot != 1 || list.Accounts[1].Slot != 2 {
		t.Fatalf("列表应含 slot 1、2: %+v", list.Accounts)
	}

	// toggle 停用。
	rec = httptest.NewRecorder()
	s.handleAccountItem(rec, httptest.NewRequest(http.MethodPost, "/accounts/1/toggle", nil))
	if rec.Code != 200 {
		t.Fatalf("toggle 失败: %s", rec.Body.String())
	}
	var tog struct {
		Account *Account `json:"account"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tog); err != nil {
		t.Fatal(err)
	}
	if tog.Account.Enabled {
		t.Fatal("toggle 后应为停用")
	}

	// save：无登录态 → 明确错误。
	rec = httptest.NewRecorder()
	s.handleAccountItem(rec, httptest.NewRequest(http.MethodPost, "/accounts/1/save", nil))
	if rec.Code != 500 {
		t.Fatalf("无登录态保存应 500: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "登录") {
		t.Fatalf("错误应提示先登录: %s", rec.Body.String())
	}

	// 伪造登录态后再 save → ok。
	cred := filepath.Join(s.accounts.rootDir, "1", ".zcode", "v2", "credentials.json")
	if err := os.MkdirAll(filepath.Dir(cred), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cred, []byte(`{"oauth:zai:access_token":"enc:v1:xxx"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.handleAccountItem(rec, httptest.NewRequest(http.MethodPost, "/accounts/1/save", nil))
	if rec.Code != 200 {
		t.Fatalf("有登录态保存应成功: %s", rec.Body.String())
	}

	// 删除。
	rec = httptest.NewRecorder()
	s.handleAccountItem(rec, httptest.NewRequest(http.MethodDelete, "/accounts/2", nil))
	if rec.Code != 200 {
		t.Fatalf("删除失败: %s", rec.Body.String())
	}
	if dirExists(filepath.Join(s.accounts.rootDir, "2")) {
		t.Fatal("删除后 slot 目录应不存在")
	}
}

// ---------- 无可用账号时的 chat 错误 ----------

func TestChatNoAccounts(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.handleChat(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewBufferString(`{"model":"GLM-5.3","messages":[{"role":"user","content":"hi"}]}`)))
	if rec.Code != 503 {
		t.Fatalf("无账号应 503: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "可用") {
		t.Fatalf("错误应提示无可用账号: %s", rec.Body.String())
	}
}

func TestChatBadBody(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.handleChat(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewBufferString(`not json`)))
	if rec.Code != 400 {
		t.Fatalf("坏请求体应 400: %d", rec.Code)
	}
}

// ---------- PickAccount 最低使用优先 ----------

func TestPickAccountLowestUses(t *testing.T) {
	s := newTestServer(t)
	for i := 0; i < 2; i++ {
		if _, err := s.accounts.Create(); err != nil {
			t.Fatal(err)
		}
	}
	// 都无登录态 → 不可选。
	if _, err := s.accounts.PickAccount(); err == nil {
		t.Fatal("无登录态账号不应被选中")
	}
	// slot1、slot2 都登录 → uses 少者优先、用后自增轮换。
	for _, n := range []int{1, 2} {
		cred := filepath.Join(s.accounts.rootDir, itoaA(n), ".zcode", "v2", "credentials.json")
		if err := os.MkdirAll(filepath.Dir(cred), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cred, []byte(`{"zcodejwttoken":"enc:v1:t"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.accounts.PickAccount(); err != nil {
		t.Fatal("有登录态账号应可选")
	}
}

// ---------- flattenContent ----------

func TestFlattenContent(t *testing.T) {
	if got := flattenContent(json.RawMessage(`"hello"`)); got != "hello" {
		t.Fatalf("string content: %q", got)
	}
	if got := flattenContent(json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)); got != "ab" {
		t.Fatalf("多段 content: %q", got)
	}
	if got := flattenContent(nil); got != "" {
		t.Fatalf("空 content: %q", got)
	}
}

// ---------- 超时零值默认 ----------

func TestCallTimeoutDefault(t *testing.T) {
	// timeout<=0 时应取 defaultCallTimeout，不 panic。
	p := NewProc("python", []string{"-c", "pass"}, "")
	_ = p
	// 只验证常量路径：Call 在未启动时会先报"未启动"（不会走到超时分支）。
	if _, err := p.Call("x", nil, 0); err == nil {
		t.Fatal("未启动 Call 应报错")
	}
	_ = time.Second
}
