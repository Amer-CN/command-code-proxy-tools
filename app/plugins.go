package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============ 开发者模式（密码门） ============

// devDefaultHash 是内置默认密码的 SHA-256 哈希（十六进制）。
// 明文密码不会出现在源码里；作者可在开发者面板中修改（哈希写入 dev_secret.json）。
// 默认密码请在构建产物交付时单独获取（不写入任何文档/源码）。
const devDefaultHash = "7d31e9d67b95ad92360d73a94629428a42b9bacb24e6607c4dfe5345d58fb8c7"

// devSecretFile 保存作者修改后的密码哈希（与 exe 同目录）。
func (a *app) devSecretFile() string { return filepath.Join(exeDir(), "dev_secret.json") }

// devVerifyPwd 校验密码：dev_secret.json 中的哈希优先，否则用内置默认哈希。
func (a *app) devVerifyPwd(pwd string) bool {
	h := sha256.Sum256([]byte(pwd))
	got := hex.EncodeToString(h[:])
	if b, err := os.ReadFile(a.devSecretFile()); err == nil {
		var v struct {
			Hash string `json:"hash"`
		}
		if json.Unmarshal(b, &v) == nil && v.Hash != "" {
			return strings.EqualFold(v.Hash, got)
		}
	}
	return strings.EqualFold(devDefaultHash, got)
}

// devSetPwd 修改密码（写入哈希文件）。
func (a *app) devSetPwd(pwd string) error {
	if len(pwd) < 4 {
		return fmt.Errorf("密码至少 4 位")
	}
	h := sha256.Sum256([]byte(pwd))
	data, _ := json.Marshal(map[string]string{"hash": hex.EncodeToString(h[:])})
	return os.WriteFile(a.devSecretFile(), data, 0o600)
}

// ============ 插件托管（Python 服务收编） ============

type pluginDef struct {
	ID     string   // 唯一标识
	Name   string   // 显示名
	Dir    string   // plugins 下目录名
	Script string   // 入口脚本
	Args   []string // 附加命令行参数
	Port   int      // 监听端口
	Health string   // 健康检查路径
}

var pluginDefs = []pluginDef{
	{
		ID: "tuanjie", Name: "团结 Cowork (Codely)",
		Dir: "tuanjie2api", Script: "codely2api.py",
		Args: []string{"--port", "8788"}, Port: 8788, Health: "/health",
	},
	{
		ID: "codebuddy", Name: "WorkBuddy / CodeBuddy",
		Dir: "codebuddy2api", Script: "converter.py",
		Args: []string{"--port", "8787"}, Port: 8787, Health: "/health",
	},
}

type pluginState struct {
	mu      sync.Mutex
	running bool
	cmd     *exec.Cmd
	started time.Time
	lastErr string
}

func (a *app) pluginsDir() string { return filepath.Join(exeDir(), "plugins") }

func (a *app) pluginStates() map[string]*pluginState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.plugins == nil {
		a.plugins = map[string]*pluginState{}
		for _, d := range pluginDefs {
			a.plugins[d.ID] = &pluginState{}
		}
	}
	return a.plugins
}

// pluginScript 返回插件入口脚本绝对路径；目录缺失时返回空。
func (a *app) pluginScript(d pluginDef) string {
	p := filepath.Join(a.pluginsDir(), d.Dir, d.Script)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// pluginHealthURL 返回插件健康检查地址。
func (a *app) pluginHealthURL(d pluginDef) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", d.Port, d.Health)
}

// pluginLog 返回插件日志路径。
func (a *app) pluginLog(d pluginDef) string {
	return filepath.Join(a.pluginsDir(), d.ID+".log")
}

// pyCmd 探测可用的 Python 命令（python / py -3），返回命令与参数前缀。
func pyCmd() (string, []string, error) {
	for _, c := range [][]string{{"python"}, {"py", "-3"}} {
		if _, err := exec.LookPath(c[0]); err == nil {
			return c[0], c[1:], nil
		}
	}
	return "", nil, fmt.Errorf("未检测到 Python，请安装 Python 3.8+（https://www.python.org/downloads/）")
}

// checkPluginDeps 检查插件依赖（fastapi / uvicorn / httpx）。
func checkPluginDeps() error {
	exe, pre, err := pyCmd()
	if err != nil {
		return err
	}
	args := append(append([]string{}, pre...), "-c", "import fastapi,uvicorn,httpx")
	cmd := exec.Command(exe, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("缺少依赖：%s\n请运行：pip install fastapi \"uvicorn[standard]\" httpx\n（详情：%s）",
			strings.TrimSpace(string(out)), err.Error())
	}
	return nil
}

// pluginList 返回所有插件的状态 JSON（目录存在/运行/健康/端口/日志）。
func (a *app) pluginList() []map[string]any {
	states := a.pluginStates()
	out := make([]map[string]any, 0, len(pluginDefs))
	for _, d := range pluginDefs {
		st := states[d.ID]
		st.mu.Lock()
		running := st.running
		lastErr := st.lastErr
		st.mu.Unlock()
		script := a.pluginScript(d)
		alive := false
		if running && script != "" {
			alive = httpOK(a.pluginHealthURL(d))
		}
		out = append(out, map[string]any{
			"id": d.ID, "name": d.Name, "port": d.Port,
			"dir": filepath.Join("plugins", d.Dir),
			"present": script != "",
			"running": running, "healthy": alive,
			"lastErr": lastErr, "log": filepath.Base(a.pluginLog(d)),
			"url": fmt.Sprintf("http://127.0.0.1:%d/v1", d.Port),
		})
	}
	return out
}

// pluginStart 启动一个插件（独立常驻子进程，关 GUI 不影响）。
func (a *app) pluginStart(id string) error {
	var d *pluginDef
	for i := range pluginDefs {
		if pluginDefs[i].ID == id {
			d = &pluginDefs[i]
			break
		}
	}
	if d == nil {
		return fmt.Errorf("未知插件: %s", id)
	}
	states := a.pluginStates()
	st := states[id]
	st.mu.Lock()
	if st.running {
		st.mu.Unlock()
		return fmt.Errorf("%s 已在运行", d.Name)
	}
	st.mu.Unlock()

	script := a.pluginScript(*d)
	if script == "" {
		return fmt.Errorf("插件目录缺失：%s\n请确认 plugins/%s 已随程序放置",
			filepath.Join(a.pluginsDir(), d.Dir), d.Dir)
	}
	if err := checkPluginDeps(); err != nil {
		return err
	}
	// 端口被非健康进程占用 → 清理（复用僵尸清理逻辑）
	if portBusy(fmt.Sprintf("%d", d.Port)) {
		_ = killByPort(fmt.Sprintf("%d", d.Port))
		time.Sleep(300 * time.Millisecond)
	}

	exe, pre, err := pyCmd()
	if err != nil {
		return err
	}
	args := append(append([]string{}, pre...), script)
	args = append(args, d.Args...)
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(script)
	if lf, err := os.OpenFile(a.pluginLog(*d), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		cmd.Stdout = lf
		cmd.Stderr = lf
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 %s 失败: %v", d.Name, err)
	}
	st.mu.Lock()
	st.running = true
	st.cmd = cmd
	st.started = time.Now()
	st.lastErr = ""
	st.mu.Unlock()

	// 等待就绪（最多 15s，uvicorn 启动较慢）
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if httpOK(a.pluginHealthURL(*d)) {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	// 启动超时不算失败：可能健康检查路径不同，留给面板状态灯判断
	return nil
}

// pluginStop 停止一个插件（按端口结束进程）。
func (a *app) pluginStop(id string) error {
	var d *pluginDef
	for i := range pluginDefs {
		if pluginDefs[i].ID == id {
			d = &pluginDefs[i]
			break
		}
	}
	if d == nil {
		return fmt.Errorf("未知插件: %s", id)
	}
	states := a.pluginStates()
	st := states[id]
	st.mu.Lock()
	if st.cmd != nil {
		_ = st.cmd.Process.Kill()
	}
	st.running = false
	st.cmd = nil
	st.lastErr = ""
	st.mu.Unlock()
	_ = killByPort(fmt.Sprintf("%d", d.Port))
	return nil
}
