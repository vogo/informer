# informer

每天把「日期信息 + 每日鸡汤 + 订阅文章推荐」汇总成一条消息，写入本地 Markdown 日报，并推送到钉钉或飞书机器人。

订阅支持四种解析方式：标准 feed（RSS/Atom）、正则匹配网页、JSON 接口，以及交给本机命令行 AI Agent 去找文章。

推荐用桌面版做日常管理。需要在服务器上定时推送时，用命令行，见 [cli-usage.md](cli-usage.md)。两者共用同一个数据目录，可以同时使用。

设计与实现约定见 [doc/design.md](doc/design.md)。

## 功能

- **订阅**：按分类管理订阅。分类用整数「排序值」决定顺序（越小越靠前，相同则按分类 ID）。卡片展示解析类型、抓取状态和错误信息，可启停、编辑、删除，也可「测试抓取」（真实请求，不写库、不改订阅状态），过程在执行日志里实时可见。
- **AI 诊断修复**：抓取失败的订阅交给 Agent 读页面、试配置。确认改动并看到复核出来的文章后，点「应用修复」才写库。
- **AI 定义配置**：新建订阅时用对话描述想订阅的内容，由 Agent 试出配置。核对提案后点「保存并新建订阅」才写库。
- **导入 / 导出**：把全部订阅配置写成 JSON，或从这样的文件合并回来，用于备份、换机器或在两台机器之间同步。
- **日报**：按「年 → 月 → 日」浏览已生成的 Markdown。
- **文章库**：按分类、订阅、关键字筛选已入库文章，显示通知时间。标题在系统浏览器中打开。
- **推送**：可手动推一次；桌面版可定时（应用打开期间）；服务器用命令行配合 crontab。目标是钉钉或飞书机器人。未配置机器人时只写日报。
- **AI 最终结果处理**：可配置提示词，对每日候选文章统一筛选、排序并生成导读，以处理结果生成正式日报和推送。支持只读测试预览。
- **设置**：抓取与推荐条数、机器人地址、HTTP 代理、Agent、AI 最终结果处理、桌面定时。也可从已有日报补回文章的通知时间。
- **运行日志**：窗口右上角查看本次运行的抓取、推送和定时任务记录。
- **应用内更新**：正式版启动时及运行期间每 24 小时检查 GitHub，自动下载并校验；版本号旁出现圆形下载箭头后，点击即可重启更新。配置与数据保留，安装目录需可写。详见[自动更新方案](doc/auto-update.md)。

## 安装

在 [Releases](https://github.com/vogo/informer/releases) 下载对应平台的安装包：

| 平台 | 手动安装 | 应用内更新使用 |
| --- | --- | --- |
| macOS | `informer-ui-<版本>-darwin-universal.dmg`（universal，未签名） | `informer-ui-<版本>-darwin-universal.app.zip` |
| Windows | `informer-ui-<版本>-windows-amd64-setup.exe`（NSIS，内置 WebView2 引导） | `informer-ui-<版本>-windows-amd64.zip` |
| Linux | `informer-ui-<版本>-linux-amd64.tar.gz` 或 `.deb` | `informer-ui-<版本>-linux-amd64.tar.gz` |

macOS 最低部署版本为 13（Go 1.27 的最低要求），兼容目标至少覆盖最近三个大版本（当前为 15、26、27）。Release 使用 macOS 15 构建 universal 包（Intel / Apple Silicon），CI 在 macOS 15 和 26 上构建并运行版本冒烟检查；完整界面功能仍需在目标系统上验证。

macOS 安装包未签名。首次打开若被拦截，到「系统设置 → 隐私与安全性」放行。

### 从源码运行桌面版

需要 [Wails v3 CLI](https://v3.wails.io/)（`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8`）、Node.js，以及 C 编译器（桌面版依赖 CGO：原生 WebView 与 mattn/go-sqlite3）：

```bash
cd cmd/informer-ui
wails3 dev      # 本地运行
wails3 build    # 出包，产物在 bin/
wails3 package  # 按当前平台打包（macOS .app / Windows NSIS / Linux 等）
```

命令行的安装见 [cli-usage.md](cli-usage.md)。

## 数据目录

`informer.json`、`informer.secret.json`、`feed.db` 和 `data/<年份>/<日期>.md` 都在同一个数据主目录：

| `INFORMER_HOME` | 数据主目录 |
| --- | --- |
| 未设置或为空 | `~/.informer`（Windows 为 `%USERPROFILE%\.informer`） |
| 已设置 | 该环境变量指向的目录（相对路径会转成绝对路径） |

数据目录与可执行文件所在位置无关。从 Finder 或桌面快捷方式启动时，用的仍是这一份数据。目录无法创建或不可写时，informer 报错退出。crontab 里的 `~` 取决于运行该任务的用户，写绝对路径更稳妥。

换目录时，先把当前数据主目录整体复制过去，再改 `INFORMER_HOME`：

```bash
cp -a ~/.informer /data/informer-home
```

## 配置

`informer.json` 是一份人类可读的 JSON，放在数据主目录下。范例见 [examples/informer.json](examples/informer.json)。桌面版在「设置」里改的就是这个文件；命令行也可以手写。

推荐参数在 `feed` 节：

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `same_site_max_count` | 同一站点在一条消息中最多出现几篇 | 必须大于 0 |
| `feed_expire_days` | 文章多少天之后视为过期，不再推荐 | 必须大于 0 |
| `max_inform_feed_size` | 一条消息最多推荐多少篇文章 | 必须大于 0 |
| `max_fetch_num` | 每个订阅默认抓取的文章数上限 | 填 0 表示不做全局限制 |

推送与代理：

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `webhooks` | 推送目标列表，每次运行全量推给所有启用的目标 | 数组，每项 `{"name": "研发群", "url": "https://...", "enabled": true}`；`url` 必填，`name` 可选（缺省显示域名），`enabled` 缺省为 `true`；不写或空数组表示只写日报、不推送 |
| `http_proxy` | HTTP(S) 代理 | 如 `http://127.0.0.1:7890`；留空或不写表示不使用代理 |

```json
{
  "webhooks": [
    {"name": "研发群", "url": "https://oapi.dingtalk.com/robot/send?access_token=xxxxx"},
    {"name": "飞书群", "url": "https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxxx", "enabled": false}
  ]
}
```

同一份消息一次并发发给所有启用的目标，部分目标失败不影响其他目标：只要至少一个目标接受，
文章照常记录通知时间；全部失败才算本次推送失败。通知时间是文章级字段，失败的目标不会被单独补推。
旧版本的单个 `webhook` 字段（无论在 `informer.json` 还是 `informer.secret.json`）仍会被读取，
下一次在设置页保存时自动迁移到 `webhooks`。

桌面版定时（命令行忽略这一节，定时仍用 crontab）：

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `schedule.enabled` | 是否启用桌面版定时推送 | `true` / `false` |
| `schedule.time` | 每日推送时间（本机时区） | 24 小时制 `"HH:MM"`，如 `"10:00"` |

应用需要保持打开。每个自然日最多成功推送一次；当天过了设定时间才打开，会补推一次。

`agent` 类型订阅使用顶层 `agent` 节：

```json
{
  "agent": {
    "provider": "claude",
    "base_url": "",
    "model": "",
    "allowed_tools": "WebSearch,WebFetch",
    "timeout_seconds": 300,
    "command": ""
  }
}
```

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `provider` | 使用哪个命令行 Agent | 目前可用 `claude`（Claude Code） |
| `base_url` | Agent 的接口地址 | 留空则沿用本机该 Agent 自己的配置 |
| `model` | 使用的模型 | 留空则沿用 Agent 的默认模型 |
| `allowed_tools` | 允许 Agent 使用的工具，逗号分隔 | 留空则使用 `WebSearch,WebFetch` |
| `timeout_seconds` | 单次运行超时 | 填 0 表示 300 秒，合法范围 10 ~ 3600 |
| `command` | Agent 可执行文件 | 留空则自动查找 `claude`；也可在设置页填写或点「自动查找」 |

API Key 不写进 `informer.json`，放在同目录的 `informer.secret.json`：

```json
{
  "agent_api_key": "sk-xxxxx"
}
```

`base_url` 和密钥都留空时，直接使用本机 Agent 已经登录的凭据。前置条件是本机已安装对应命令行（例如 `claude --version` 能正常输出）。设置页只显示密钥「是否已配置」，不回传原文。

### AI 最终结果处理

在「设置 → AI 最终结果处理」开启并填写提示词，或在 `informer.json` 添加：

```json
{
  "result_processing": {
    "enabled": true,
    "prompt": "按技术深度、实用性和信息增量综合判断，只保留评分达到 80 分的文章；过滤广告和重复资讯，生成简洁中文导读；没有合格文章则跳过推送。"
  }
}
```

`enabled` 缺省为 `false`；开启时 `prompt` 必填。阈值是提示词中的模型判断规则，不是程序的客观质量评分。模型、凭据、代理和超时沿用 `agent` 设置；此阶段禁用工具，仅根据标题、来源域名和发布时间分析，不读取正文或进行事实核验。

每次运行在现有规则选出的候选范围内统一处理一次，不补选文章。处理后的导读和精选文章成为正式日报，所有启用的机器人接收同一内容。全部过滤则正常跳过本次推送、不生成新日报，桌面定时当天不再重试；处理失败或输出不合法则报错，不退回原始内容推送，桌面定时按失败重试。关闭功能时保留提示词，恢复原有日报格式。

「测试处理」使用当前提示词、已保存的 Agent 设置与文章库中尚未通知的候选，不保存表单、不抓取、不生成日报、不推送，也不修改文章通知状态；没有候选时需先准备未通知的文章。预览显示原始候选、最终内容和逐篇筛选原因。

正式运行的输入、提示词、模型原始返回、筛选原因及处理错误保存在数据主目录的 `runs/<日期>/processing-*.json`（0600），与日报目录分开。被过滤的文章保持未通知，后续运行可重新参与筛选；只对最终选中且成功推送的文章记录通知状态。无机器人时，成功保存正式日报即记录。处理失败、全部过滤、日报保存失败或全部机器人失败均不记录。

添加 agent 订阅的命令见 [cli-usage.md](cli-usage.md#agent-解析订阅)。配置如何落盘见 [doc/design.md](doc/design.md#配置文件)。

## 桌面版

窗口顶部四个 Tab：

| Tab | 怎么用 |
| --- | --- |
| **订阅** | 左侧管分类，右侧是该分类下的订阅卡片。右上角导出 / 导入。新建弹窗右上角是「AI 定义配置」。 |
| **日报** | 左侧点某一天，右侧渲染当天的 Markdown。 |
| **文章库** | 筛选后翻页浏览。点标题在系统浏览器中打开。 |
| **设置** | 改参数、机器人、代理和定时，手动推送一次，或重建历史索引。 |

窗口右上角「日志」查看本次运行记录，可按级别和关键字筛选、复制、清空。日志不落盘，重启后清空。

### AI 诊断修复

抓取失败的卡片上点「AI 诊断修复」。抽屉列出每个字段的「原值 → 新值」和复核解析出的标题。确认是文章之后点「应用修复」。复核没通过时没有这个按钮。通常要一到几分钟，底部有诊断日志。

### AI 定义配置

在新建订阅弹窗里打开。说清楚想订阅哪里的什么内容，Agent 试出配置后，聊天里出现「保存并新建订阅」。可以改标题、选分类或输入新分类名。agent 类型的提案会标成「未复核」，保存后请用卡片上的「测试抓取」再验一次。

### 导出 / 导入

「导出」写出全部订阅（与当前分类筛选无关），分类按名字。「导入」按 URL 认已有订阅（没有 URL 的按标题）：对得上的覆盖配置，对不上的追加，不删除现有订阅。文件里的分类名本机没有时会自动创建。某一条不完整会被跳过，并在结果里列出来。

### 重建历史索引

「设置 → 重建历史索引」用已有日报里的文章链接，给库中通知时间为空的文章补上日期。可以重复执行。

## 命令行

服务器定时推送、或不便开图形界面时，使用 `informer`。安装、订阅管理和 crontab 见 [cli-usage.md](cli-usage.md)。

Release 里的命令行包名为 `informer-cli-<版本>-<平台>.tar.gz`（Windows 为 `.zip`），解压后命令为 `informer`。

## 参与开发

```bash
make test      # 跑测试并生成覆盖率
make format    # 按 golangci-lint 配置格式化
make lint-update # 安装/更新最新版 golangci-lint
make lint      # 检查根模块和桌面模块（Linux 自动使用 gtk3）
make check     # 许可证头检查 + 两个模块的 lint
```

许可证：[Apache License 2.0](LICENSE)。
