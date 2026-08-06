# CommandCode Proxy Deck v2.0 — 本地 Agent 执行指令

> 把本文件保存到仓库根目录 `F:\AIXM\command\本地Agent指令.md`，然后对本地 agent 说：「读 本地Agent指令.md，严格按步骤执行，每步汇报结果」。

## 背景

- 仓库：`F:\AIXM\command`（GitHub: `Amer-CN/command-code-proxy-tools`，Go module: `github.com/dev2k6/command-code-proxy-server`）
- 这是一个 Windows 本地代理工具：把 CommandCode 的 CLI 内部接口转成 OpenAI 兼容接口 `http://127.0.0.1:55990/v1`。
- v2.0 已把旧的 HTA 控制台推翻，重做为 **WebView2 独立 GUI 程序**。新代码已放入仓库目录，你的任务是：**编译、修复、验证、提交**。

## 已就位的新文件（如缺失立即报告，不要自己补写）

| 文件 | 作用 |
|---|---|
| `app/main.go` | WebView2 窗口入口（webview_go 纯 Go 绑定，内嵌 ui.html，进程内 HTTP 提供给 WebView2） |
| `app/bridge.go` | JS↔Go 桥：进程内启停代理核心、状态/统计/模型透传、开机自启、剪贴板、杀软提示持久化 |
| `app/platform_windows.go` | 深色标题栏 / DPI 感知 / 原生消息框 |
| `app/ui.html` | 科幻全息控制台界面（星空、3D 反应堆核心、统计动画、模型矩阵、日志流） |
| `构建EXE.bat`、`启动代理.bat`、`停止代理.bat`、`设置开机自启.bat`、`应用更新.bat` | 构建与运维脚本 |
| `README.md`、`RELEASE_NOTES.md`、`使用说明.txt`、`.gitignore` | 已重写/更新 |

以下旧文件应已不存在（若存在则删除）：`CommandCode代理-单文件版.hta`、`hta_template.txt`、`生成单文件版.py`、`bin/` 整个目录。

**约束：修复编译错误只允许改 `app/` 下的文件；`internal/` 是上游代理核心，一律不动。**

## 执行步骤

### 1. 环境检查
- `go version`。未安装 → `winget install GoLang.Go`；版本低于 go.mod 声明的 `go 1.26.2` → 保持 `GOTOOLCHAIN=auto`（默认）让 go 自动下载对应工具链；若自动下载失败，可把 go.mod 的 `go` 指令降到本机版本（最低 1.22）再试。
- `dir app` 应有 4 个文件；确认旧文件已删除。

### 2. 拉取依赖
```bat
go get github.com/webview/webview_go@latest github.com/atotto/clipboard@latest
go mod tidy
```

### 3. 编译
```bat
set CGO_ENABLED=0
go build -trimpath -ldflags="-H windowsgui -s -w" -o CommandCodeProxyDeck.exe ./app
```
- 若 `app/` 编译报错（最可能是 webview_go 版本 API 变动）：到 `%GOPATH%\pkg\mod\github.com\webview\` 下读该库实际源码对齐。期望的 API 形态：`webview.NewWithOptions(webview.WebViewOptions{Debug, AutoFocus, WindowOptions: webview.WindowOptions{Title, Width, Height, Center}})` 返回 nil-able WebView；方法 `Run/Destroy/Window() uintptr/Dispatch(func())/SetSize(w,h,webview.HintMin)/Navigate(url)/Bind(name, func) error`。
- 编译成功应得到约 10~16MB 的单个 exe。

### 4. 单元测试
```bat
go test ./...
```
上游 `internal/proxy` 的测试应全绿；与 app/ 无关的失败原样报告即可。

### 5. 功能验证（逐项执行并记录）
1. 运行 `CommandCodeProxyDeck.exe` → 出现 1280×800 深色科幻窗口（开机自检动画 → 三栏布局：左侧能量核心 / 中间链路与统计 / 右侧模型矩阵 / 底部日志）。
2. 点击能量核心（或按空格）→ 核心变翠绿脉冲；另开终端 `curl http://127.0.0.1:55990/health` 返回 `{"status":"ok"}`；`curl http://127.0.0.1:55990/v1/models` 返回 18 个模型。
3. 界面「模型矩阵」点击任一模型名 → 剪贴板应得到模型 ID（粘贴验证）。
4. 再点核心停堆 → `curl /health` 应连接失败。
5. 关闭窗口。
6. 无窗口模式：`start "" CommandCodeProxyDeck.exe -headless` → `curl /health` 应 OK → `taskkill /F /IM CommandCodeProxyDeck.exe` 清理。
7. 外部实例容错：先 `go run . -port 55990` 占用端口（根目录旧 CLI 仍在），再开 exe 点核心 → 界面应进入「外部实例」态且能停止它；测完清理进程。

### 6. 提交
```bat
git add -A
git commit -m "v2.0.0: WebView2 独立程序 + 科幻全息控制台"
git push
```
`api-key.txt`、`stats.json`、`notice_dismissed.flag`、`headless-error.log`、`CommandCodeProxyDeck.exe` 均已在 .gitignore 中，不应被提交。

### 7. 汇报格式
- Go 版本 / 依赖版本（webview_go、clipboard 的实际版本号）
- 编译产物大小
- `go test ./...` 结果
- 功能验证 1~7 逐项通过/失败（失败的附报错原文）
- 提交哈希

## 注意事项
- bat 里中文若在 GBK 终端显示乱码，属编码问题，不影响功能；直接用本文档里的命令行操作即可，不必依赖 bat。
- 运行 exe 若 SmartScreen / 杀软提示，选「仍要运行 / 允许」。
- 若提示缺少 WebView2 运行时（极罕见），从 https://developer.microsoft.com/zh-cn/microsoft-edge/webview2/ 安装后重试。
- 完成后可删除 `应用更新.bat` 和本指令文件。
