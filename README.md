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
- **设置**：抓取与推荐条数、机器人地址、HTTP 代理、Agent、桌面定时。也可从已有日报补回文章的通知时间。
- **运行日志**：窗口右上角查看本次运行的抓取、推送和定时任务记录。
- **应用内更新**：正式发布版发现新版本后，可下载并重启生效。

## 安装

在 [Releases](https://github.com/vogo/informer/releases) 下载对应平台的安装包：

| 平台 | 手动安装 | 应用内更新使用 |
| --- | --- | --- |
| macOS | `informer-ui-<版本>-darwin-universal.dmg`（universal，未签名） | `informer-ui-<版本>-darwin-universal.app.zip` |
| Windows | `informer-ui-<版本>-windows-amd64-setup.exe`（NSIS，内置 WebView2 引导） | `informer-ui-<版本>-windows-amd64.zip` |
| Linux | `informer-ui-<版本>-linux-amd64.tar.gz` 或 `.deb` | `informer-ui-<版本>-linux-amd64.tar.gz` |

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
| `webhook` | 钉钉或飞书机器人地址 | URL；留空或不写表示只写日报、不推送 |
| `http_proxy` | HTTP(S) 代理 | 如 `http://127.0.0.1:7890`；留空或不写表示不使用代理 |

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
make check     # 许可证头检查 + golangci-lint
```

许可证：[Apache License 2.0](LICENSE)。
