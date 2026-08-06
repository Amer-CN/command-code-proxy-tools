# CommandCode 代理控制台（Windows 独立程序 · WebView2）

给 [CommandCode](https://commandcode.ai) 用户准备的 Windows 本地代理工具：
把官方只对 **Provider 套餐**开放的接口，转成 OpenAI 兼容的本地接口
`http://127.0.0.1:55990/v1`，**普通套餐即可使用**。

v2.0 起，控制台从 HTA 全面重做：**科幻全息界面的独立 exe 程序**，代理核心直接在程序进程内运行，双击即用、点火秒开。

v2.1 起新增**官网权威统计**：直连 CommandCode 官方账单接口，金额 / 总 Token / 运行次数 /
套餐额度全部以官网为准（不再依赖本地估算），仅需在控制台填一次 API Key。

> 代理核心来自 [dev2k6/command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)（上游 v1.0.9）。
> 上游原始 README 存档于 [UPSTREAM_README.md](UPSTREAM_README.md)。

## 为什么需要它

- CommandCode 官方 Provider API（`/provider/v1`）要求 **Provider 套餐**，普通套餐直接调用会被拒绝
- 本代理走 CommandCode CLI 使用的内部接口（`/alpha/generate`），**普通套餐就能用**
- 任何 OpenAI 兼容客户端 / Agent（Codex、Cherry Studio、NextChat、OpenCode 等）填上本地地址即可使用

## 快速开始

```bat
:: 1. 构建（需要本机装有 Go 1.22+，https://go.dev/dl/）
构建EXE.bat

:: 2. 得到 CommandCodeProxyDeck.exe，双击打开
:: 3. 点击左侧「能量核心」点火（或按空格）
```

然后在任意 Agent / 客户端中配置：

- **Base URL**：`http://127.0.0.1:55990/v1`
- **API Key**：你在 CommandCode Studio 生成的 Key（登录 <https://commandcode.ai/studio/> → API keys → Generate API key）

选择模型即可使用（18 个模型，控制台「模型矩阵」里点击模型名一键复制）。

## 界面与功能

| 区域 | 说明 |
|---|---|
| 代理核心 | 3D 全息反应堆，**点击点火 / 停堆**（快捷键空格）；点火琥珀色充能、运行翠绿脉冲、故障红色告警 |
| 链路配置 | Base URL 一键复制；API Key 输入即自动保存（或点「保存」按钮），留空则用客户端自己的 Key |
| 链路监测 | **官网权威统计**：金额 / 总消耗 / 运行次数直接来自官方账单接口（标注「官网」），每 8 秒自动刷新；今日与分模型明细本地兜底 |
| 模型矩阵 | 18 个模型按厂商分组（DeepSeek / Kimi / GLM / Qwen / Gemini / MiniMax / 阶跃 / MiMo），点击复制 |
| 运行日志 | 启停与错误事件流，分级着色；「开机自启」开关也在这里 |
| 氛围 | 星空 + 透视网格地板 + 星云 + 扫描线背景，面板随鼠标 3D 视差，开机自检动画 |

其他细节：深色沉浸式标题栏（Win10 1809+）、高 DPI 自适应、关窗自动优雅停堆、
检测到开机自启的后台实例时自动接入监测（外部实例态，可一键停止）。

### 官网统计说明（v2.1+）

- 金额卡片显示**官网账单金额**（如 `$2.07`），副标题给出可用额度与已用百分比；总消耗与运行次数同为官网口径
- 官网接口结果缓存 20 秒，轮询不卡界面；离线时自动回退本地统计（标注「本地估算」）
- 个人账号（无组织）自动直调，无需任何额外配置

## 命令行用法

```bat
CommandCodeProxyDeck.exe                  :: 打开控制台（GUI）
CommandCodeProxyDeck.exe -headless        :: 无窗口后台模式（开机自启用的就是这个）
CommandCodeProxyDeck.exe -port 55990      :: 指定端口（默认 55990）
CommandCodeProxyDeck.exe -api-key <KEY>   :: 指定默认 API Key
启动代理.bat / 停止代理.bat               :: 命令行启停（后台模式）
设置开机自启.bat / 取消开机自启.bat       :: 登录 Windows 自动后台运行
```

## 技术架构

```
CommandCodeProxyDeck.exe（单个独立 exe，~15MB）
├─ app/                  WebView2 窗口 + JS 桥（github.com/webview/webview_go，纯 Go，无 CGO/DLL）
│   ├─ ui.html           内嵌界面（go:embed，经 127.0.0.1 随机端口提供给 WebView2）
│   ├─ bridge.go         启停控制 / 状态 / 统计透传 / 开机自启 / 剪贴板
│   └─ platform_windows.go  深色标题栏 / DPI / 消息框
└─ internal/proxy + internal/server   上游代理核心，进程内直接运行
```

- 界面依赖系统自带的 **WebView2 运行时**（Win10/11 随系统/Edge 预装），所以工具体积不膨胀
- 对比旧 HTA 版：不再需要释放内置 exe（13MB → 单程序秒开）、不再依赖 mshta/ActiveX/JScript
- 数据文件（均与 exe 同目录）：`api-key.txt`（记住的 Key）、`stats.json`（本地统计后备，旧版 `bin\stats.json` 会自动迁移）

## 性能说明

- 官网统计接口结果**缓存 20 秒**：GUI 每 8 秒轮询时直接读缓存（毫秒级），不阻塞界面
- 界面日志流有上限（220 条自动裁剪），长时间运行不因 DOM 膨胀变卡
- 统计写入为原子写（tmp + rename），对话中每轮写一次，无锁竞争问题

## 常见问题（FAQ）

### 报错 `400 ... expected number to be <=200000 at "params.max_tokens"`，我是不是被封了？

**不是被封号。** 这是请求参数校验错误（封号/欠费会是 401/403/429）。部分 Agent 会按模型上下文窗口发送超大 `max_tokens`，超过接口上限 200,000 被拒绝。**v1.0.1 起代理已自动钳制**，请求全部合法化，输出空间不受影响（API 上限本就是 20 万）。

### SmartScreen 或杀软（火绒/360）提示？

本程序是未签名的开源工具，首次运行 SmartScreen 选「更多信息 → 仍要运行」；「开机自启」会向启动文件夹写入脚本，杀软询问时选允许即可。代码完全开源，可自行审计后用 `构建EXE.bat` 重新编译。

### 提示缺少 WebView2 运行时？

极少见（被精简系统卸载了）。程序会弹出提示并打开官方下载页，安装后即可使用。

### 为什么不能直接用官方文档里的接口？

官方 `/provider/v1` 需要 Provider 套餐；本工具走 CLI 内部接口，普通套餐可用——这也是本工具存在的意义。

### 支持哪些系统？

仅 Windows。使用需有自己的 CommandCode 账号与套餐（普通套餐即可）。

## 更新日志

见 [RELEASE_NOTES.md](RELEASE_NOTES.md)。

## 致谢

- [dev2k6/command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)
- 模型与额度：由你的 CommandCode 账号提供
