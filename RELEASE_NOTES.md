# CommandCode 代理控制台 / Windows Launcher Tools

给 [command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)（原作者 **dev2k6**，本仓库核心代码来自上游）
做的 Windows 便携工具：本地代理 + HTML 控制台 + 一键启停 + 单文件版。

## 文件说明

| 文件 | 用途 |
|---|---|
| `CommandCode代理-单文件版.hta` | **单文件版**：双击即用，自动释放内置代理组件并启动（约 13MB） |
| `CommandCode控制台.hta` | 多文件版控制台（依赖 `bin\` 下的 exe） |
| `启动代理.bat` / `停止代理.bat` | 命令行启停 |
| `设置开机自启.bat` / `取消开机自启.bat` | 开机自动后台运行 |
| `生成单文件版.py` + `hta_template.txt` | 重新打包单文件版的构建工具 |

## 使用

1. 打开控制台 → 点「启动代理」（或直接运行 `启动代理.bat`）
2. 在任意 OpenAI 兼容客户端 / Agent 里配置：
   - Base URL: `http://127.0.0.1:55990/v1`
   - API Key: 你自己在 CommandCode Studio 生成的 Key
3. 可用 17 个模型：DeepSeek V4、Kimi K2.6、GLM-5.1、Qwen 3.7、Gemini 3.1 Flash-Lite、MiniMax、小米 MiMo 等（控制台里点模型名可复制）

> 代理会把 OpenAI 格式请求转发到 CommandCode 的 `/alpha/generate`（CLI 内部接口），
> 因此**不需要 Provider 套餐**，普通套餐即可使用。

## 注意

- 仅支持 Windows（HTA 依赖系统自带 mshta）
- 杀软（火绒/360 等）可能对"内嵌可执行文件的 HTA / 写启动项的脚本"报警，
  首次使用请选择允许，或把文件夹加入信任区
- 需要你自己的 CommandCode 账号与套餐

## 致谢

代理核心来自 [dev2k6/command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)。
