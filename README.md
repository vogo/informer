# informer

每天把「日期信息 + 每日鸡汤 + 订阅文章推荐」汇总成一条消息，写入本地 Markdown 日报，并推送到钉钉或飞书机器人。

订阅源支持四种解析方式：标准 feed（RSS/Atom）、正则匹配网页、JSON 接口，以及交给命令行 AI Agent 去找。

推荐使用桌面版：图形化管理分类与订阅、浏览日报、检索文章库、配置参数与机器人。需要在服务器上定时推送时，再用命令行，见 [cli-usage.md](cli-usage.md)。两者共用同一个数据目录和 Service 层，可以同时使用。

## 一、桌面版（推荐）

`cmd/informer-ui` 是 Wails v3 + Vue 3 + Naive UI 的桌面入口。窗口顶部分为四个 Tab：

| Tab | 能做什么 |
| --- | --- |
| **订阅** | 左侧分类树：新增 / 编辑 / 删除分类，用整数「排序值」决定顺序（越小越靠前，相同排序值按分类 ID 排）。右侧按分类筛选的订阅卡片：展示解析类型、抓取状态、所属分类、错误信息，卡片右上角的开关直接启停订阅，底部可「测试抓取」（真实抓取，但不写库、不改订阅状态）、编辑、删除。测试抓取的抽屉底部有「执行日志」折叠面板，抓取过程中实时输出：请求地址与 HTTP 状态码、正则/JSON 路径的匹配情况、逐条解析出的标题与链接；agent 类型还会显示完整提示词、agent 每一次搜索与网页抓取、以及返回的原文。抓取中面板自动展开（日志就是进度），成功后自动折叠，失败则保持展开。右上角可「导出 / 导入」全部订阅配置，见下文。 |
| **日报** | 左侧按「年 → 月 → 日」折叠的日期列表，点击某天在右侧全宽渲染当天的 Markdown。单日内容一次性加载，不分页。 |
| **文章库** | 已入库文章的游标翻页列表，可按分类、订阅、关键字筛选，显示通知时间；标题点击后在系统浏览器中打开。 |
| **设置** | 读写 `informer.json` 的抓取与推荐参数、配置定时推送（仅桌面端，应用打开时生效）、手动触发一次推送、配置机器人地址与 HTTP 代理（写入 `informer.json`）、执行「重建历史索引」。 |

窗口右上角的「日志」按钮打开**系统运行日志**面板，见下文。

正式发布构建会在启动时及之后每 24 小时检查一次 GitHub Releases；若有新版本则后台下载并校验，右上角出现「重启生效新版本」，点击后替换二进制并重启。开发版（`version=dev`）不会发起检查。

### 系统运行日志

桌面版从 Finder 或开始菜单启动，stdout 没有人看得到：凌晨定时推送失败，第二天没有任何痕迹可查。所以进程把自己写出的每一行日志同时留一份在内存里（默认最多 2000 行，stdout 照旧输出），窗口右上角的「日志」按钮打开面板即可翻看：

- **内容**：定时任务的触发与结果、每次抓取的请求与解析、推送到机器人的结果、启动失败的原因——即订阅页「测试抓取」里那种执行日志，加上不属于任何一次测试抓取的后台运行记录。
- **筛选**：面板顶部工具条按级别（全部 / 警告与错误 / 仅错误）和关键字过滤；「复制」复制当前筛选出的日志，方便贴进 issue，「清空」丢掉已有的行只看接下来发生的事。
- **跟随**：面板打开时每秒读取一次新增行并滚到底部；向上滚动即暂停跟随，回到底部恢复。也可以关掉「自动刷新」，改用「刷新」按钮手动读。
- **边界**：日志只活在本次运行的内存里，**不落盘**，重启应用后清空；超过上限时最早的行被覆盖，面板会明确写出「已省略较早的 N 条」而不是假装日志是连续的。启动失败时这个面板照常可用——那正是最需要它的时候。

### 安装

在仓库的 [Releases](https://github.com/vogo/informer/releases) 页面下载最新版本对应平台的安装包：

| 平台 | 手动安装 | 应用内更新使用 |
| --- | --- | --- |
| macOS | `informer-ui-<版本>-darwin-universal.dmg`（universal，未签名） | `informer-ui-<版本>-darwin-universal.app.zip` |
| Windows | `informer-ui-<版本>-windows-amd64-setup.exe`（NSIS，内置 WebView2 引导） | `informer-ui-<版本>-windows-amd64.zip` |
| Linux | `informer-ui-<版本>-linux-amd64.tar.gz` 或 `.deb` | `informer-ui-<版本>-linux-amd64.tar.gz` |

macOS 安装包未做代码签名与公证，首次打开（以及自更新替换后）可能需在「系统设置 → 隐私与安全性」中放行。

### 从源码运行

需要安装 [Wails v3 CLI](https://v3.wails.io/)（`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8`）、Node.js 与 C 编译器（桌面版依赖 CGO：原生 WebView 与 mattn/go-sqlite3）：

```bash
cd cmd/informer-ui
wails3 dev      # 本地运行
wails3 build    # 出包，产物在 bin/
wails3 package  # 按当前平台打包（macOS .app / Windows NSIS / Linux 等）
```

### 导出 / 导入订阅

订阅页右上角的「导出」把**全部**订阅（不受左侧分类和过滤条件影响）写成一个 JSON 文件，「导入」再把这样的文件合并回来，用于换机器、备份或在两台机器之间同步订阅：

- **导出的内容**：每个订阅的完整配置（标题、URL、curl、解析类型及其参数、Agent 提示词、抓取选项、启用状态），分类**按名字**导出。数据库自增 ID 和抓取状态（正常 / 失败及错误信息）不导出，它们只属于本机。
- **唯一键**：有 URL 的订阅按 URL 认，没有 URL 的（Agent 订阅本来就没有地址）按标题认。
- **导入是「追加 + 覆盖」**：能对上唯一键的订阅覆盖其配置（本机的抓取状态保留），对不上的追加为新订阅，**不会删除**任何订阅。因此导入一个只含几条订阅的文件，只会新增和更新这几条。
- **分类自动补齐**：文件里的分类名本机没有时自动创建；分类名为空则归入「未分类」。
- **单条失败不影响整体**：某一条既没有 URL 也没有标题、或 Agent 订阅缺提示词等，会被跳过并在结果里逐条列出，其余照常导入。
- 文件版本比当前程序新时整体拒绝导入，以免把不认识的字段悄悄丢掉；手写文件也可以直接用一个 JSON 数组（不带外层对象）。

### 重建历史索引

如果库里有一批文章的通知时间（`informed_at`）是空的，「设置 → 重建历史索引」可以从已经生成的日报 Markdown 里把这部分信息补回来：

- **判定依据只有链接**：从日报里提取文章链接，与库中文章的 URL 做**精确**匹配。只有「恰好匹配到一条、且该文章的通知时间为空」
  才会补值，补的是该日报**当天的本地 00:00**（精度到天，因为文件名只能证明到天）。
- **不覆盖、不编造**：已经有通知时间的文章原样保留；库里找不到的链接、匹配到多条文章的链接，一律跳过并保持为空。
- **可重复执行**：结果幂等，第二次运行只会把第一次补好的记录报告为「已有时间」。
- 执行后会给出扫描天数、链接数，以及成功 / 跳过（分「已有时间」「库中无此链接」「匹配到多条」）/ 失败的数量。

### 几条与数据安全相关的实现约定

- 日报 Markdown 以关闭原始 HTML 的方式渲染，并在注入 DOM 前做一次消毒；报告里的链接一律走系统浏览器打开，不会让 WebView 自身跳转。
- 日期只接受 `2006-01-02` 这一种写法并由后端校验，界面无法读到数据目录以外的文件。
- 文章库翻页用文章 ID 作游标而不是 offset，定时任务在翻页期间插入新文章也不会造成重复或漏项；切换筛选条件会回到第一页。

### 当前不支持

不做拖拽排序、不做全文搜索；配置不迁移到数据库、不提供配置的版本历史或回滚，也不支持多用户 / 多环境配置。

## 二、数据目录

informer 的所有数据（`informer.json`、`informer.secret.json`、`feed.db`、`data/<年份>/<日期>.md`）都存放在**同一个数据主目录**中：

| `INFORMER_HOME` | 数据主目录 |
| --- | --- |
| 未设置或为空 | `~/.informer`（Windows 为 `%USERPROFILE%\.informer`） |
| 已设置 | 该环境变量指向的目录（相对路径会转成绝对路径） |

数据目录与可执行文件所在位置无关，因此从 Finder / 桌面快捷方式启动（不继承 shell 环境变量）时，找到的是同一份数据。
如果 `INFORMER_HOME` 指向的目录无法创建或不可写，informer 会直接报错退出，**不会**退回到另一份数据继续跑。
注意 crontab 中 `~` 取决于运行该任务的用户，显式写绝对路径更稳妥。

切换到另一个目录时，先把当前数据主目录整体复制过去，再修改 `INFORMER_HOME`：

```bash
cp -a ~/.informer /data/informer-home
# 然后把 crontab 里的 INFORMER_HOME 改成 /data/informer-home
```

informer 不会自动同步多个数据主目录，也不支持同时使用多个 `INFORMER_HOME`。

## 三、配置文件

`informer.json` 是一份人类可读的整份 JSON，不入库、不做版本历史。范例见 [examples/informer.json](examples/informer.json)。

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `same_site_max_count` | 同一站点在一条消息中最多出现几篇 | 必须大于 0 |
| `feed_expire_days` | 文章多少天之后视为过期，不再推荐 | 必须大于 0 |
| `max_inform_feed_size` | 一条消息最多推荐多少篇文章 | 必须大于 0 |
| `max_fetch_num` | 每个订阅默认抓取的文章数上限 | 填 0 表示不做全局限制 |

前三项为 0 会让推荐算法选不出任何文章，因此被拒绝；超出合理范围的值同样会被拒绝，且被拒绝的保存不会改动文件。

还有一个顶层字段 `webhook`，存放钉钉 / 飞书机器人地址（不是敏感凭证）：

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `webhook` | 推送用的机器人 webhook | URL 字符串；留空或不写表示不推送 |

还有一个顶层字段 `http_proxy`，供 URL 抓取、桌面版应用内更新与 Agent 子进程共用：

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `http_proxy` | HTTP(S) 代理地址 | 如 `http://127.0.0.1:7890`；留空或不写表示不使用代理 |

有配置时，订阅抓取、每日鸡汤、机器人推送与桌面版检查/下载新版本会经共享 HTTP 客户端走代理；Agent（如 `claude`）子进程会注入 `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY`。这与 `agent.base_url`（API 网关地址）无关。

还有一个顶层节 `schedule`，只被桌面版「定时任务」读取，命令行与系统 crontab 会忽略它（命令行的定时推送仍由 crontab 负责）：

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `schedule.enabled` | 是否启用桌面版定时推送 | `true` / `false` |
| `schedule.time` | 每日推送时间（本机时区） | 24 小时制 `"HH:MM"`，如 `"10:00"` |

桌面版定时仅在应用保持打开时生效，每个自然日最多成功推送一次（记录在数据目录的 `informer.schedule-state`，重启同一天不会再补推；失败会在下次轮询重试）；当天过了设定时间才打开应用，会补推一次。

除 `feed` 节外还有一个顶层节 `agent`，供 `agent` 类型的订阅使用：

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
| `provider` | 使用哪个命令行 Agent | 目前只实现了 `claude`（Claude Code）；`codex` 已预留但本版本运行时会报错 |
| `base_url` | Agent 的接口地址 | 留空表示沿用本机上该 Agent 自己的配置 |
| `model` | 使用的模型 | 留空表示沿用 Agent 的默认模型 |
| `allowed_tools` | 允许 Agent 使用的工具，逗号分隔 | 留空使用默认 `WebSearch,WebFetch` |
| `timeout_seconds` | 单次运行超时 | 填 0 表示使用默认 300 秒，合法范围 10 ~ 3600 |
| `command` | Agent 可执行文件 | 留空时会在 PATH、常见安装目录与登录 Shell 中自动查找 `claude`/`codex`，找到后写入配置；也可在设置页手动填写或点「自动查找」 |

API Key 不写进 `informer.json`，而是放在同目录的 `informer.secret.json` 的 `agent_api_key` 字段里。
`base_url` 与 `agent_api_key` 都留空时，informer 直接运行本机 Agent，用的就是该 Agent 自身已登录的凭据——
也就是说，只要本机 `claude`（或对应 Agent）能跑，agent 订阅就能跑，不需要额外配置。

桌面版从 Dock / Finder 启动时往往没有终端里的 PATH。`command` 留空时，informer 会在 PATH、Homebrew / npm / nvm 等常见目录以及登录 Shell 里查找可执行文件，并把绝对路径写回 `informer.json`；设置页也可以手动填写或点「自动查找」。

前置条件：本机已安装对应命令行 Agent（例如 `claude --version` 能正常输出）。命令行添加 agent 订阅的方式见 [cli-usage.md](cli-usage.md#agent-解析订阅)。

无论是命令行手写还是桌面版「设置」页保存，写入都遵守下面几条约定：

- **保留未知字段**：保存只替换 `feed`、`agent`、`schedule`、`webhook` 与 `http_proxy`，文件里其它顶层字段（包括这个版本还不认识的字段）连同顺序一起原样保留。
- **原子替换**：先写同目录下的临时文件再 `rename` 覆盖，所以并发运行的 crontab 读到的要么是旧文件、要么是新文件，不会读到写了一半的内容。
- **跨进程写锁**：写入前会创建 `informer.json.lock`，两个「读—改—写」不会互相覆盖造成丢失更新。锁有等待上限，
  等不到会直接报错而不是一直卡住；进程被杀留下的陈旧锁会在明显过期后被自动接管。

### 敏感配置文件 informer.secret.json

Agent API Key 不写进 `informer.json`，而是单独存放在数据主目录下的 `informer.secret.json`：

```json
{
  "agent_api_key": "sk-xxxxx"
}
```

- 该文件无论新建还是改写，权限都会被设置并校验为 **0600**；权限无法落实时保存直接失败，不会以不安全的状态写下去
  （Windows 不支持 Unix 权限位，这一步在 Windows 上无法校验）。
- 桌面版「设置」页只显示「是否已配置」，不回传原文。
- 机器人 webhook 写在 `informer.json` 里，不进入这个文件；若旧版本曾把 webhook 写在这里，运行时仍会读取，并在下次保存地址时迁出。

## 四、命令行

服务器定时推送、或不方便开图形界面时，使用命令行版 `informer`。安装、订阅管理、crontab 见 **[cli-usage.md](cli-usage.md)**。

Release 里的命令行包名为 `informer-cli-<版本>-<平台>.tar.gz`（Windows 为 `.zip`），解压后命令为 `informer`。

## 五、参与开发

```bash
make test      # 跑测试并生成覆盖率
make format    # 按 golangci-lint 配置格式化
make check     # 许可证头检查 + golangci-lint
```

打 `v*` 标签后由 `.github/workflows/release.yml` 在 macOS / Windows / Linux 三个原生 runner 上构建桌面版与命令行版，
并聚合发布到 GitHub Release（含桌面安装包与应用内更新用的 zip/tar.gz，命令行 `informer-cli` 压缩包，以及固定名 `SHA256SUMS`）。
当前阶段不做代码签名 / 公证。

许可证：[Apache License 2.0](LICENSE)。
