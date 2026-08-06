# CommandCode 代理控制台（Windows 便携工具）

给 [CommandCode](https://commandcode.ai) 用户准备的 Windows 本地代理工具包：
把官方只对 **Provider 套餐**开放的接口，转成 OpenAI 兼容的本地接口
`http://127.0.0.1:55990/v1`，**普通套餐即可使用**，并附 HTML 控制台、一键启停、开机自启、单文件便携版。

> 代理核心来自 [dev2k6/command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)（上游 v1.0.8）。
> 本仓库在保留上游全部代码与历史的基础上，新增了 Windows 便携工具与修复（见[更新日志](#更新日志)）。
> 上游原始 README 存档于 [UPSTREAM_README.md](UPSTREAM_README.md)。

## 为什么需要它

- CommandCode 官方 Provider API（`/provider/v1`）要求 **Provider 套餐**，普通套餐直接调用会被拒绝
- 本代理走 CommandCode CLI 使用的内部接口（`/alpha/generate`），**普通套餐就能用**
- 任何 OpenAI 兼容客户端 / Agent（Codex、Cherry Studio、NextChat、OpenCode 等）填上本地地址即可使用

## 快速开始

1. 从 [Releases](https://github.com/Amer-CN/command-code-proxy-tools/releases) 下载（二选一）：
   - `CommandCode代理-单文件版.hta`：**一个文件搞定**。双击即用，自动释放内置代理组件并启动（首次约 10~60 秒）
   - `CommandCode代理-便携版.zip`：解压即用，包含控制台与启停脚本
2. 在任意 Agent / 客户端中配置：
   - **Base URL**：`http://127.0.0.1:55990/v1`
   - **API Key**：你在 CommandCode Studio 生成的 Key（登录 https://commandcode.ai/studio/ → API keys → Generate API key）
3. 选择模型即可使用（支持 17 个模型，见下表）

## 可用模型

| 模型（可填别名或全名） | 说明 |
|---|---|
| `deepseek-v4-pro` / `deepseek-v4-flash` | DeepSeek V4 系列，1M 上下文 |
| `kimi-k2.6` / `kimi-k2.5` | Moonshot Kimi |
| `glm-5.1` / `glm-5` | 智谱 GLM |
| `qwen-3.6-max` / `qwen-3.7-max` / `qwen-3.7-max-free` | 通义千问 |
| `gemini-3.1-flash-lite` | Google Gemini |
| `minimax-m3` / `minimax-m2.7` / `minimax-m2.5` | MiniMax |
| `step-3.7-flash` / `step-3.5-flash` | 阶跃星辰 |
| `mimo-v2.5` / `mimo-v2.5-pro` | 小米 MiMo |

完整列表与短别名见控制台内「可用模型」区（点击模型名一键复制）。

## 文件说明

| 文件 | 用途 |
|---|---|
| `CommandCode代理-单文件版.hta` | 单文件版：内嵌代理组件，双击即用 |
| `CommandCode控制台.hta` | 多文件版控制台：启停/状态/模型列表/开机自启 |
| `启动代理.bat` / `停止代理.bat` | 命令行启停 |
| `设置开机自启.bat` / `取消开机自启.bat` | 开机自动后台运行 |
| `生成单文件版.py` + `hta_template.txt` | 重新打包单文件版的构建工具 |

## 常见问题（FAQ）

### 报错 `400 ... expected number to be <=200000 at "params.max_tokens"`，我是不是被封了？

**不是被封号。** 这是**请求参数校验错误**，与账号、用量、封号无关（封号/欠费会是 401/403/429）。

原因：部分 Agent（如 Codex）**无法设置"最大输出 token"**，会按模型上下文窗口（如 1M）发送超大 `max_tokens`（如 100 万），超过 CommandCode 接口上限 **200,000** 而被直接拒绝，表现为"任务突然中断"。

**v1.0.1 起代理已自动钳制** `max_tokens` 到 200000（即 API 上限），请求全部合法化，不再需要手动设置。输出空间不受任何影响（API 上限本就是 20 万），正常请求完全无感。

### 钳制 max_tokens 会不会截断长任务的输出？

不会。`max_tokens` 是"单次回复的输出上限"，200000 就是该 API 的最大允许值；钳制只是让请求合法，并不会截断任何输出。正常任务（几百~几千 token）完全不受影响，需要超长输出时依然能用到满额 20 万。

### 杀毒软件报警或文件被清理？

单文件版"内嵌可执行文件"、开机自启脚本"写入启动项"容易被火绒/360 等安全软件判定为可疑。首次使用若弹窗请选择"允许/信任"，或把工具目录加入杀软信任区；文件被清理的话，加完信任区重新下载/生成即可。

### 为什么不能直接用官方文档里的接口？

官方 `/provider/v1` 需要 Provider 套餐；本工具走 CLI 内部接口，普通套餐可用——这也是本工具存在的意义。

### 支持哪些系统？

仅 Windows（HTA 依赖系统自带 mshta 引擎）。使用需有自己的 CommandCode 账号与套餐（普通套餐即可）。

## 更新日志

### v1.0.1（2026-08-06）
- **修复**：`max_tokens` 超限导致的 400 错误——代理层统一钳制到 200000（API 上限），并新增单元测试
- 单文件版 / 多文件版同步更新（内嵌修复后的代理）

### v1.0.0（2026-08-06）
- 首个发布：单文件版 HTA、HTML 控制台、启停/开机自启脚本、单文件构建工具

## 致谢

- 代理核心：[dev2k6/command-code-proxy-server](https://github.com/dev2k6/command-code-proxy-server)
- 模型与额度：由你的 CommandCode 账号提供
