// accounts.go —— 多账号 slot 管理：目录、启动登录实例、校验登录态、列表。
// slot 目录：exe同目录/zcode-accounts/<N>/（N=1,2,3…），元数据 <N>/account.json。
package zcoderemote

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Account 是一个账号 slot 的元数据（落盘 account.json + API 返回结构）。
type Account struct {
	Slot        int    `json:"slot"`
	Name        string `json:"name"`
	CreatedAt   string `json:"createdAt"`
	LastLoginAt string `json:"lastLoginAt,omitempty"`
	Enabled     bool   `json:"enabled"`
	HasLogin    bool   `json:"hasLogin"`
	Uses        int64  `json:"uses"` // 转发选中次数（round-robin 参考，最低使用优先）
}

// zcodeExeCandidates 是 ZCode 桌面端 exe 候选路径。
var zcodeExeCandidates = []string{
	`D:\Program Files\ZCode\ZCode.exe`,
}

// DetectZCodeExe 探测 ZCode 桌面端 exe：写死路径 → 运行中进程 Path。
func DetectZCodeExe() (string, error) {
	for _, p := range zcodeExeCandidates {
		if fileExists(p) {
			return p, nil
		}
	}
	for _, p := range runningZCodePaths() {
		if fileExists(p) {
			return p, nil
		}
	}
	return "", errors.New("未找到 ZCode 桌面端（ZCode.exe）。请先安装 ZCode")
}

// Accounts 管理全部 slot（线程安全）。
type Accounts struct {
	mu      sync.Mutex
	rootDir string
	procs   map[int]*Proc // 每个账号的无头 app-server（懒启动+复用）
	rrLast  int           // round-robin 游标
}

// NewAccounts 创建管理器。rootDir 为 slot 根目录（exe同目录/zcode-accounts）。
func NewAccounts(rootDir string) *Accounts {
	return &Accounts{
		rootDir: rootDir,
		procs:   map[int]*Proc{},
	}
}

// DefaultAccountsRoot 返回 exe 同目录下的 slot 根目录。
func DefaultAccountsRoot() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "zcode-accounts")
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "zcode-accounts")
	}
	return "zcode-accounts"
}

// slotDir 返回第 N 个 slot 的目录。
func (a *Accounts) slotDir(n int) string {
	return filepath.Join(a.rootDir, itoaA(n))
}

// metaPath 返回第 N 个 slot 的 account.json 路径。
func (a *Accounts) metaPath(n int) string {
	return filepath.Join(a.slotDir(n), "account.json")
}

// List 返回全部账号（按 slot 升序）。目录存在即列出（元数据缺失时补默认值）。
func (a *Accounts) List() []*Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	ents, err := os.ReadDir(a.rootDir)
	if err != nil {
		return nil
	}
	var out []*Account
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(e.Name(), "%d", &n); err != nil || n < 1 || itoaA(n) != e.Name() {
			continue
		}
		out = append(out, a.loadMetaLocked(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// loadMetaLocked 读取（或补建）slot N 的元数据。调用方持锁。
func (a *Accounts) loadMetaLocked(n int) *Account {
	ac := &Account{Slot: n, Name: "账号 " + itoaA(n), Enabled: true}
	b, err := os.ReadFile(a.metaPath(n))
	if err == nil {
		_ = json.Unmarshal(b, ac)
	}
	ac.Slot = n
	if ac.Name == "" {
		ac.Name = "账号 " + itoaA(n)
	}
	// hasLogin 以落盘登录态实时为准：凭据文件在即视为已登录
	// （保存登录态写 true，凭据被删/未登录自动回落 false）。
	ac.HasLogin = a.hasLoginFilesLocked(n)
	return ac
}

// saveMetaLocked 落盘 slot N 的元数据。调用方持锁。
func (a *Accounts) saveMetaLocked(ac *Account) error {
	if err := os.MkdirAll(a.slotDir(ac.Slot), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ac, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.metaPath(ac.Slot), b, 0o644)
}

// hasLoginFilesLocked 检查 slot 目录下的登录态凭据（两处都查）：
//   - .zcode/v2/credentials.json（app-server 主登录态，含 oauth token）
//   - appdata/Roaming/ZCode/…（Electron 桌面端数据，--user-data-dir 重定向）
func (a *Accounts) hasLoginFilesLocked(n int) bool {
	return len(a.credentialFiles(a.slotDir(n))) > 0
}

// credentialFiles 返回 slot 目录下所有含 oauth token 的凭据文件路径。
func (a *Accounts) credentialFiles(slotDir string) []string {
	var found []string
	// 1. .zcode/v2/credentials.json（键 oauth:zai:access_token 等已加密凭据）
	cred := filepath.Join(slotDir, ".zcode", "v2", "credentials.json")
	if b, err := os.ReadFile(cred); err == nil && hasZcodeToken(b) {
		found = append(found, cred)
	}
	// 2. appdata/Roaming/ZCode（Electron --user-data-dir 重定向后的登录态）
	roaming := filepath.Join(slotDir, "appdata", "Roaming", "ZCode")
	if dirExists(roaming) {
		// rum-electron-store / Local State 里的 session 数据存在即认为桌面端落过盘
		if fileExists(filepath.Join(roaming, "Local State")) ||
			dirExists(filepath.Join(roaming, "rum-electron-store")) ||
			dirExists(filepath.Join(roaming, "session")) {
			found = append(found, roaming)
		}
	}
	return found
}

// hasZcodeToken 判断 credentials.json 内容里是否有 ZCode oauth 键
// （真实值为 enc:v1: 加密串，只匹配键名，不解密）。
func hasZcodeToken(b []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	for k, v := range m {
		if (strings.HasPrefix(k, "oauth:zai:") || k == "zcodejwttoken") && len(v) > 4 {
			return true
		}
	}
	return false
}

// Create 新建一个 slot（取当前最大 N + 1），返回元数据。
func (a *Accounts) Create() (*Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(a.rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建账号目录失败: %w", err)
	}
	max := 0
	ents, _ := os.ReadDir(a.rootDir)
	for _, e := range ents {
		var n int
		if _, err := fmt.Sscanf(e.Name(), "%d", &n); err == nil && n > max {
			max = n
		}
	}
	n := max + 1
	if err := os.MkdirAll(a.slotDir(n), 0o755); err != nil {
		return nil, fmt.Errorf("创建 slot 目录失败: %w", err)
	}
	ac := &Account{Slot: n, Name: "账号 " + itoaA(n), Enabled: true, CreatedAt: time.Now().Format(time.RFC3339)}
	if err := a.saveMetaLocked(ac); err != nil {
		return nil, err
	}
	log.Printf("[zcoderemote] 新建账号 slot=%d dir=%s", n, a.slotDir(n))
	return ac, nil
}

// Get 返回指定 slot 的元数据；不存在返回 nil。
func (a *Accounts) Get(n int) *Account {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !dirExists(a.slotDir(n)) {
		return nil
	}
	return a.loadMetaLocked(n)
}

// Delete 删除一个 slot（停掉其 app-server 后整目录删除，不可逆）。
func (a *Accounts) Delete(n int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !dirExists(a.slotDir(n)) {
		return fmt.Errorf("账号 %d 不存在", n)
	}
	if p := a.procs[n]; p != nil {
		_ = p.Close()
		delete(a.procs, n)
	}
	if err := os.RemoveAll(a.slotDir(n)); err != nil {
		return fmt.Errorf("删除 slot 目录失败: %w", err)
	}
	log.Printf("[zcoderemote] 删除账号 slot=%d", n)
	return nil
}

// Toggle 切换启用/停用，返回新状态。
func (a *Accounts) Toggle(n int) (*Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !dirExists(a.slotDir(n)) {
		return nil, fmt.Errorf("账号 %d 不存在", n)
	}
	ac := a.loadMetaLocked(n)
	ac.Enabled = !ac.Enabled
	if err := a.saveMetaLocked(ac); err != nil {
		return nil, err
	}
	// 停用即停该账号的无头 app-server（下次启用再懒启动）。
	if !ac.Enabled {
		if p := a.procs[n]; p != nil {
			_ = p.Close()
			delete(a.procs, n)
		}
	}
	log.Printf("[zcoderemote] 账号 %d enabled=%v", n, ac.Enabled)
	return ac, nil
}

// LaunchLoginInstance 用 USERPROFILE=<slot目录> + --user-data-dir=<slot>/appdata
// 启动完整 ZCode 桌面端（detached，日志不接管），用户在里面登录目标账号。
func (a *Accounts) LaunchLoginInstance(n int) error {
	exe, err := DetectZCodeExe()
	if err != nil {
		return err
	}
	a.mu.Lock()
	slot := a.slotDir(n)
	a.mu.Unlock()
	if !dirExists(slot) {
		return fmt.Errorf("账号 %d 不存在", n)
	}
	if err := os.MkdirAll(filepath.Join(slot, "appdata"), 0o755); err != nil {
		return fmt.Errorf("创建 appdata 目录失败: %w", err)
	}
	cmd := exec.Command(exe, "--user-data-dir="+filepath.Join(slot, "appdata"))
	// USERPROFILE 重定向：app-server/桌面端在该目录下自建全新 .zcode 结构。
	cmd.Env = append(os.Environ(), "USERPROFILE="+slot)
	// detached：Windows 下不设 SysProcAttr，Start 后立即返回（GUI 进程独立生存）。
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 ZCode 登录实例失败: %w", err)
	}
	log.Printf("[zcoderemote] 已启动登录实例 slot=%d exe=%s pid=%d", n, exe, cmd.Process.Pid)
	_ = cmd.Process.Release()
	return nil
}

// SaveLogin 校验 slot 登录态（凭据文件含 oauth token）并落盘 hasLogin。
func (a *Accounts) SaveLogin(n int) (*Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !dirExists(a.slotDir(n)) {
		return nil, fmt.Errorf("账号 %d 不存在", n)
	}
	files := a.credentialFiles(a.slotDir(n))
	ac := a.loadMetaLocked(n)
	if len(files) == 0 {
		ac.HasLogin = false
		if err := a.saveMetaLocked(ac); err != nil {
			return nil, err
		}
		return nil, errors.New("未检测到登录态：请先启动登录实例并在其中登录目标账号，完成后再保存")
	}
	ac.HasLogin = true
	ac.LastLoginAt = time.Now().Format(time.RFC3339)
	if err := a.saveMetaLocked(ac); err != nil {
		return nil, err
	}
	log.Printf("[zcoderemote] 登录态已保存 slot=%d files=%v", n, files)
	return ac, nil
}

// ProcFor 返回账号的无头 app-server Proc（懒启动+复用；崩溃自动重启一次）。
func (a *Accounts) ProcFor(n int) (*Proc, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	slot := a.slotDir(n)
	if !dirExists(slot) {
		return nil, fmt.Errorf("账号 %d 不存在", n)
	}
	p := a.procs[n]
	if p != nil && p.Alive() {
		return p, nil
	}
	// 崩溃残留 → 清理后重启一次。
	if p != nil {
		_ = p.Close()
		delete(a.procs, n)
	}
	np, err := NewSlotProc(slot)
	if err != nil {
		return nil, err
	}
	if err := np.Start(); err != nil {
		return nil, err
	}
	a.procs[n] = np
	log.Printf("[zcoderemote] app-server 已启动 slot=%d", n)
	return np, nil
}

// PickAccount 选一个可转发账号（enabled 且 hasLogin，最低使用优先）。
// 无可用账号时返回错误。
func (a *Accounts) PickAccount() (*Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var cand []*Account
	ents, err := os.ReadDir(a.rootDir)
	if err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			var n int
			if _, err := fmt.Sscanf(e.Name(), "%d", &n); err != nil || n < 1 || itoaA(n) != e.Name() {
				continue
			}
			ac := a.loadMetaLocked(n)
			if ac.Enabled && ac.HasLogin {
				cand = append(cand, ac)
			}
		}
	}
	if len(cand) == 0 {
		return nil, errors.New("没有可用的 ZCode 账号：请在「多开」视图添加账号、登录并启用")
	}
	sort.Slice(cand, func(i, j int) bool {
		if cand[i].Uses != cand[j].Uses {
			return cand[i].Uses < cand[j].Uses
		}
		return cand[i].Slot < cand[j].Slot
	})
	pick := cand[0]
	pick.Uses++
	_ = a.saveMetaLocked(pick) // 使用次数持久化（失败无碍转发）
	return pick, nil
}

// ProcStates 返回各账号 app-server 进程状态（/health 展示）。
func (a *Accounts) ProcStates() []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []map[string]any{}
	for n, p := range a.procs {
		out = append(out, map[string]any{
			"slot":  n,
			"alive": p.Alive(),
		})
	}
	return out
}

// CloseAll 停掉全部无头 app-server（服务退出时调用）。
func (a *Accounts) CloseAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for n, p := range a.procs {
		_ = p.Close()
		delete(a.procs, n)
	}
}
