# informer

每天把「日期信息 + 每日鸡汤 + 订阅文章推荐」汇总成一条消息，写入本地 Markdown 日报，并推送到钉钉或飞书机器人。

订阅源支持四种解析方式：标准 feed（RSS/Atom）、正则匹配网页、JSON 接口，以及交给命令行 AI Agent 去找。

## 一、选择使用方式

| 方式 | 适合 | 能做什么 |
| --- | --- | --- |
| 命令行 `informer` | 服务器上定时推送 | 管理订阅、抓取文章、生成日报、推送机器人 |
| 桌面版 `informer-ui` | 本机图形化管理 | 分类与订阅维护、日报浏览、文章库检索、参数与机器人配置 |

两者共用同一个数据目录和 Service 层，可以同时使用：用桌面版维护订阅和参数，用命令行做定时推送。

## 二、快速开始（命令行）

### 1. 安装

需要 Go 1.25 或更高版本：

```bash
GOBIN=$(pwd) go install github.com/vogo/informer/cmd/informer@master
```

`cmd/informer` 是命令行入口；仓库根包是等价的入口，`go install github.com/vogo/informer@master` 得到的是同一个程序。
命令行版本可在 `CGO_ENABLED=0` 下构建，适合放到服务器上跑。

### 2. 创建配置文件

推送运行时**必须**能读到 `informer.json`。把它放在数据主目录下（默认路径 `~/.informer/informer.json`）：

```bash
mkdir -p ~/.informer
cat > ~/.informer/informer.json <<'EOF'
{
  "feed": {
    "same_site_max_count": 3,
    "feed_expire_days": 150,
    "max_inform_feed_size": 20,
    "max_fetch_num": 1
  }
}
EOF
```

各项含义与取值约束见下文[配置文件](#四配置文件)。也可以装好桌面版后在「设置」Tab 里填写保存，写的是同一个文件。

### 3. 添加订阅

```bash
# 添加订阅，输出为「id, 标题, 地址」
./informer feed add "阮一峰blog" http://www.ruanyifeng.com/blog/atom.xml
# 1,	阮一峰blog,	http://www.ruanyifeng.com/blog/atom.xml

# 测试抓取（只解析并打印，不写库、不改订阅状态）
./informer feed parse 1
# 科技爱好者周刊（第 243 期）：与孔子 AI 聊天 : http://www.ruanyifeng.com/blog/2023/02/weekly-issue-243.html
# 科技爱好者周刊（第 242 期）：一次尴尬的服务器被黑 : http://www.ruanyifeng.com/blog/2023/02/weekly-issue-242.html
```

### 4. 配置机器人

在钉钉或飞书中创建群机器人，安全设置选「自定义关键词」，得到 webhook 地址：

- 钉钉：`https://oapi.dingtalk.com/robot/send?access_token=xxxxx`
- 飞书：`https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxxx`

地址可以在命令行显式传入，也可以保存到[敏感配置文件](#敏感配置文件-informersecretjson)后不带参数运行。

### 5. 执行一次推送

```bash
./informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx"
```

命令行传入的地址优先；不传时使用 `informer.secret.json` 里保存的 webhook。两处都没有地址时，
只生成内容并写入日报文件，不推送。

### 6. 配置定时任务

crontab 不会加载 shell 的 profile，需要在命令里显式写环境变量和绝对路径。每天早上 10 点推送：

```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx" >> /root/informer/cron.log 2>&1
```

飞书同理，把地址换成飞书的 webhook 即可。地址已保存在 `informer.secret.json` 时可以省略参数：

```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer >> /root/informer/cron.log 2>&1
```

## 三、数据目录

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

## 四、配置文件

`informer.json` 是一份人类可读的整份 JSON，不入库、不做版本历史。范例见 [examples/informer.json](examples/informer.json)。

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `same_site_max_count` | 同一站点在一条消息中最多出现几篇 | 必须大于 0 |
| `feed_expire_days` | 文章多少天之后视为过期，不再推荐 | 必须大于 0 |
| `max_inform_feed_size` | 一条消息最多推荐多少篇文章 | 必须大于 0 |
| `max_fetch_num` | 每个订阅默认抓取的文章数上限 | 填 0 表示不做全局限制 |

前三项为 0 会让推荐算法选不出任何文章，因此被拒绝；超出合理范围的值同样会被拒绝，且被拒绝的保存不会改动文件。

除 `feed` 节外还有一个顶层节 `agent`，供 `agent` 类型的订阅使用，详见[「agent 解析订阅」](#agent-解析订阅)。

还有一个顶层节 `schedule`，只被桌面版「定时任务」读取，命令行与系统 crontab 会忽略它（命令行的定时推送仍由 crontab 负责）：

| 配置项 | 含义 | 取值 |
| --- | --- | --- |
| `schedule.enabled` | 是否启用桌面版定时推送 | `true` / `false` |
| `schedule.time` | 每日推送时间（本机时区） | 24 小时制 `"HH:MM"`，如 `"10:00"` |

桌面版定时仅在应用保持打开时生效，每个自然日最多成功推送一次（记录在数据目录的 `informer.schedule-state`，重启同一天不会再补推；失败会在下次轮询重试）；当天过了设定时间才打开应用，会补推一次。

无论是命令行手写还是桌面版「设置」页保存，写入都遵守下面几条约定：

- **保留未知字段**：保存只替换 `feed`、`agent` 与 `schedule` 三节，文件里其它顶层字段（包括这个版本还不认识的字段）连同顺序一起原样保留。
- **原子替换**：先写同目录下的临时文件再 `rename` 覆盖，所以并发运行的 crontab 读到的要么是旧文件、要么是新文件，不会读到写了一半的内容。
- **跨进程写锁**：写入前会创建 `informer.json.lock`，两个「读—改—写」不会互相覆盖造成丢失更新。锁有等待上限，
  等不到会直接报错而不是一直卡住；进程被杀留下的陈旧锁会在明显过期后被自动接管。

### 敏感配置文件 informer.secret.json

机器人地址（webhook）不写进 `informer.json`，而是单独存放在数据主目录下的 `informer.secret.json`：

```json
{
  "webhook": "https://oapi.dingtalk.com/robot/send?access_token=xxxxx",
  "agent_api_key": "sk-xxxxx"
}
```

- 该文件无论新建还是改写，权限都会被设置并校验为 **0600**；权限无法落实时保存直接失败，不会以不安全的状态写下去
  （Windows 不支持 Unix 权限位，这一步在 Windows 上无法校验）。
- 桌面版「设置」页只显示「是否已配置」和一段打码后的前缀，完整地址不会回传到界面。
- 命令行显式传入的地址**优先**；不传地址时才使用这个文件里保存的 webhook。
- `agent_api_key` 是 `agent` 类型订阅调用 AI Agent 时使用的密钥，同样只存在这个文件里；桌面版只显示「是否已配置」，不回传原文。

## 五、订阅管理命令

```bash
./informer feed list              # 列出所有订阅（id, 标题, 地址）
./informer feed view <id>         # 查看单个订阅的全部字段
./informer feed add <标题> <地址>   # 添加订阅
./informer feed agent <标题> <提示词>  # 添加 agent 类型订阅（不需要地址）
./informer feed copy <id>         # 复制一个订阅（连同其设置）
./informer feed remove <id>       # 删除订阅
./informer feed update <id> <字段> <值>   # 修改订阅的某个字段
./informer feed parse <id>        # 测试抓取，只打印不写库
./informer feed category          # 列出分类（id, 名称, 排序值）
```

常用字段：

| 字段 | 说明 |
| --- | --- |
| `title` / `url` | 订阅标题与抓取地址 |
| `weight` | 文章排序权重，越大越靠前 |
| `max_fetch_num` | 该订阅单次抓取的文章数上限 |
| `parse_type` | 解析方式，合法值 `feed` / `regex` / `json` / `agent` |
| `category_id` | 所属分类，默认 `1`（内置「未分类」，不可删除） |
| `enabled` | 是否参与定时抓取；停用后 `feed parse` 仍可测试 |
| `regex` / `title_exp` / `url_exp` | 正则解析用的匹配式与标题、链接表达式 |
| `json_title_path` / `json_url_path` | JSON 解析用的标题、链接路径 |
| `agent_prompt` / `agent_provider` | agent 解析用的提示词与指定的 Agent |
| `redirect` | 是否跟随解析出的链接跳转 |

示例：

```bash
./informer feed update 1 weight 80          # 提高排序权重
./informer feed update 1 max_fetch_num 1    # 每次只取 1 篇
./informer feed update 1 category_id 2      # 归入分类 2
./informer feed update 1 enabled false      # 停用订阅
```

### 正则解析订阅

对于不提供 feed 的网页，用正则从 HTML 中提取标题和链接：

```bash
./informer feed add "Julian Shapiro blog" https://www.julian.com/
./informer feed update 14 parse_type regex
./informer feed update 14 regex '<a href="([^"]+)" class="blog-post-link[^"]+"><div class="blog-post-link-text">([^<]+)</div>'
./informer feed update 14 title_exp '$2'
./informer feed update 14 url_exp 'https://www.julian.com$1'

# 测试抓取
./informer feed parse 14
# Armageddon : https://www.julian.com/blog/armageddon
# What to do with your life : https://www.julian.com/blog/life-planning
```

`title_exp` 和 `url_exp` 用 `$1`、`$2` 引用 `regex` 中的捕获分组，可与固定前缀拼接成完整链接。

### agent 解析订阅

`agent` 类型的订阅不抓取某个固定地址，而是把一段自然语言的任务交给本机安装的命令行 AI Agent 去找文章。
**你只需要描述要找什么**，JSON 输出格式要求由 informer 自动追加到提示词末尾，返回内容再被解析成文章列表：

```bash
./informer feed agent "Go 语言周报" "搜索 Go 语言最近的技术文章与发布公告，挑出最值得阅读的几篇，给出标题和链接。"
# 1,	Go 语言周报,	agent

./informer feed update 1 max_fetch_num 3   # 最多要 3 条，这个数字也会写进提示词

# 测试抓取（真的会调用一次 Agent，可能需要几十秒）
./informer feed parse 1
```

几点约定：

- 订阅本身**不需要 `url`**；`agent_prompt` 为空的 agent 订阅会被拒绝保存。
- 返回条目里 `title` 为空、或 `url` 不是 `http://` / `https://` 开头的会被丢弃，重复链接只保留一条，
  所以 Agent 给出相对路径或编造格式时不会污染文章库。
- Agent 只被允许使用只读的联网工具（默认 `WebSearch,WebFetch`），并且整次运行有超时上限，
  超时会被杀掉并把原因记到订阅的抓取状态里，不会拖住整轮抓取。

Agent 的接口地址、密钥与模型是全局配置，写在 `informer.json` 的 `agent` 节（密钥除外）：

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
| `command` | Agent 可执行文件 | 留空表示用 `PATH` 里的 `claude` |

API Key 不写进 `informer.json`，而是放在同目录的 `informer.secret.json` 的 `agent_api_key` 字段里。
`base_url` 与 `agent_api_key` 都留空时，informer 直接运行 `claude`，用的就是本机 `claude` 自身已登录的凭据——
也就是说，只要本机 `claude` 能跑，agent 订阅就能跑，不需要额外配置。

前置条件：本机已安装 Claude Code 命令行（`claude --version` 能正常输出）。

## 六、桌面版 informer-ui

`cmd/informer-ui` 是 Wails v2 + Vue 3 + Naive UI 的桌面入口。窗口顶部分为四个 Tab：

| Tab | 能做什么 |
| --- | --- |
| **订阅** | 左侧分类树：新增 / 编辑 / 删除分类，用整数「排序值」决定顺序（越小越靠前，相同排序值按分类 ID 排）。右侧按分类筛选的订阅卡片：展示解析类型、抓取状态、所属分类、错误信息，卡片右上角的开关直接启停订阅，底部可「测试抓取」（真实抓取，但不写库、不改订阅状态）、编辑、删除。 |
| **日报** | 左侧按「年 → 月 → 日」折叠的日期列表，点击某天在右侧全宽渲染当天的 Markdown。单日内容一次性加载，不分页。 |
| **文章库** | 已入库文章的游标翻页列表，可按分类、订阅、关键字筛选，显示通知时间；标题点击后在系统浏览器中打开。 |
| **设置** | 读写 `informer.json` 的抓取与推荐参数、配置定时推送（仅桌面端，应用打开时生效）、手动触发一次推送、配置机器人地址（写入独立的敏感文件）、执行「重建历史索引」。 |

### 安装

在仓库的 [Releases](https://github.com/vogo/informer/releases) 页面下载最新版本对应平台的安装包：

| 平台 | 安装包 |
| --- | --- |
| macOS | `informer-ui-<版本>-darwin-universal.dmg`（universal，未签名） |
| Windows | `informer-ui-<版本>-windows-amd64-setup.exe`（NSIS，内置 WebView2 引导） |
| Linux | `informer-ui-<版本>-linux-amd64.tar.gz` 或 `.deb` |

macOS 安装包未做代码签名与公证，首次打开需在「系统设置 → 隐私与安全性」中放行。

### 从源码运行

需要安装 [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation)、Node.js 与 C 编译器（桌面版依赖 CGO：原生 WebView 与 mattn/go-sqlite3）：

```bash
cd cmd/informer-ui
wails dev      # 本地运行
wails build    # 出包，产物在 build/bin/
```

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

不做拖拽排序、不做订阅批量导入导出、不做全文搜索；配置不迁移到数据库、不提供配置的版本历史或回滚，也不支持多用户 / 多环境配置。

## 七、参与开发

```bash
make test      # 跑测试并生成覆盖率
make format    # 按 golangci-lint 配置格式化
make check     # 许可证头检查 + golangci-lint
```

打 `v*` 标签后由 `.github/workflows/release.yml` 在 macOS / Windows / Linux 三个原生 runner 上构建桌面版，
并聚合发布到 GitHub Release（macOS 为未签名 universal dmg，Windows 为内置 WebView2 引导的 NSIS 安装包，
Linux 为 x86_64 tar.gz 与 deb）。当前阶段不做代码签名 / 公证。

许可证：[Apache License 2.0](LICENSE)。
