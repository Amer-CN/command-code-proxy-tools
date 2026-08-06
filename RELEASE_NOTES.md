# CommandCode 代理控制台 / Windows Launcher Tools

给 [command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)（原作者 **dev2k6**，本仓库核心代码来自上游）
做的 Windows 便携工具：本地代理 + 科幻全息控制台 + 一键启停 + 开机自启。

## v2.0.0（2026-08-06）—— WebView2 独立程序 · 界面全部重做

- **全新控制台**：HTA 退役，改为正式的 Windows 独立程序 `CommandCodeProxyDeck.exe`（WebView2 渲染）
  - 科幻全息界面：3D 反应堆核心（点击点火/停堆，空格键亦可）、星空/透视网格/星云/扫描线背景、
    玻璃拟态面板 + 鼠标 3D 视差、Toast 操作反馈、开机自检动画
  - 统计卡片数字滚动动画 + 分模型消耗条形图；模型矩阵按厂商分组、点击复制
  - 深色沉浸式标题栏、高 DPI 自适应、关窗自动优雅停堆
- **架构重做**：代理核心直接在主程序进程内运行（不再释放/调用 `bin\` 下的 exe）——点火秒开，无解压等待
- **体积不变大**：WebView2 运行时随系统/Edge 预装，无需打包进 exe
- 新增 `-headless` 无窗口后台模式（开机自启由此驱动）
- 新增外部实例检测：开机自启的后台代理已在跑时，控制台自动接入监测，可一键停止
- 新增 `构建EXE.bat` 一键构建脚本（自动拉依赖 + 编译出单个 exe）
- 旧版 `bin\stats.json` 统计数据自动迁移到 exe 同目录
- **移除**：`CommandCode代理-单文件版.hta`、`hta_template.txt`、`生成单文件版.py`、`bin/` 预编译产物
  （均仍存在于 git 历史与旧 Release 中）
- ⚠️ **迁移提示**：旧版开机自启脚本指向已删除的 `bin\command-code-proxy.exe`，
  升级后请在新控制台里重新开关一次「开机自启」（或运行一次新的 设置开机自启.bat）

## v1.0.1（2026-08-06）

- **修复**：`max_tokens` 超限导致的 400 错误——代理层统一钳制到 200000（API 上限），并新增单元测试
- 单文件版 / 多文件版同步更新（内嵌修复后的代理）

## v1.0.0（2026-08-06）

- 首个发布：单文件版 HTA、HTML 控制台、启停/开机自启脚本、单文件构建工具

## 使用

1. 运行 `构建EXE.bat` 生成 `CommandCodeProxyDeck.exe`，双击打开 → 点核心点火
2. 在任意 OpenAI 兼容客户端 / Agent 里配置：
   - Base URL: `http://127.0.0.1:55990/v1`
   - API Key: 你自己在 CommandCode Studio 生成的 Key
3. 可用 18 个模型：DeepSeek V4、Kimi K2.6、GLM-5.1、Qwen 3.7、Gemini 3.1 Flash-Lite、MiniMax、小米 MiMo 等

> 代理会把 OpenAI 格式请求转发到 CommandCode 的 `/alpha/generate`（CLI 内部接口），
> 因此**不需要 Provider 套餐**，普通套餐即可使用。

## 注意

- 仅支持 Windows；界面依赖系统自带的 WebView2 运行时（一般已预装）
- 未签名程序，SmartScreen / 杀软首次提示请选「仍要运行 / 允许」
- 需要你自己的 CommandCode 账号与套餐

## 致谢

代理核心来自 [dev2k6/command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)。
