# 命令行 informer

命令行适合在服务器上定时抓取、生成日报并推送到钉钉 / 飞书。配置文件、数据目录与桌面版完全相同，可以一边用桌面版维护订阅，一边用命令行做 crontab 推送。数据目录与 `informer.json` 的字段说明见 [README](README.md)。

## 安装

### 预编译包（推荐）

在仓库的 [Releases](https://github.com/vogo/informer/releases) 页面下载对应平台的 `informer-cli` 压缩包，解压后命令为 `informer`：

| 平台 | 包名 | 解压后 |
| --- | --- | --- |
| macOS | `informer-cli-<版本>-darwin-universal.tar.gz` | `informer` |
| Linux amd64 | `informer-cli-<版本>-linux-amd64.tar.gz` | `informer` |
| Linux arm64 | `informer-cli-<版本>-linux-arm64.tar.gz` | `informer` |
| Windows amd64 | `informer-cli-<版本>-windows-amd64.zip` | `informer.exe` |

```bash
tar -xzf informer-cli-<版本>-linux-amd64.tar.gz
./informer feed list
```

Windows 解压 zip 后得到 `informer.exe`，在 PowerShell / cmd 里同样以 `informer` 调用。

### 从源码安装

需要 Go 1.25 或更高版本，以及本机 C 编译器（sqlite 依赖 CGO）：

```bash
GOBIN=$(pwd) go install github.com/vogo/informer/cmd/informer@master
```

`cmd/informer` 是命令行入口；仓库根包是等价的入口，`go install github.com/vogo/informer@master` 得到的是同一个程序。

## 快速开始

### 1. 创建配置文件

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

各项含义与取值约束见 [README 配置文件](README.md#三配置文件)。也可以装好桌面版后在「设置」Tab 里填写保存，写的是同一个文件。

### 2. 添加订阅

```bash
# 添加订阅，输出为「id, 标题, 地址」
./informer feed add "阮一峰blog" http://www.ruanyifeng.com/blog/atom.xml
# 1,	阮一峰blog,	http://www.ruanyifeng.com/blog/atom.xml

# 测试抓取（只解析并打印，不写库、不改订阅状态）
./informer feed parse 1
# 科技爱好者周刊（第 243 期）：与孔子 AI 聊天 : http://www.ruanyifeng.com/blog/2023/02/weekly-issue-243.html
# 科技爱好者周刊（第 242 期）：一次尴尬的服务器被黑 : http://www.ruanyifeng.com/blog/2023/02/weekly-issue-242.html
```

### 3. 配置机器人

在钉钉或飞书中创建群机器人，安全设置选「自定义关键词」，得到 webhook 地址：

- 钉钉：`https://oapi.dingtalk.com/robot/send?access_token=xxxxx`
- 飞书：`https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxxx`

地址可以在命令行显式传入，也可以保存到 `informer.json` 的顶层 `webhook` 字段后不带参数运行。

### 4. 执行一次推送

```bash
./informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx"
```

命令行传入的地址优先；不传时使用 `informer.json` 里保存的 webhook。两处都没有地址时，
只生成内容并写入日报文件，不推送。

### 5. 配置定时任务

crontab 不会加载 shell 的 profile，需要在命令里显式写环境变量和绝对路径。每天早上 10 点推送：

```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx" >> /root/informer/cron.log 2>&1
```

飞书同理，把地址换成飞书的 webhook 即可。地址已保存在 `informer.json` 时可以省略参数：

```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer >> /root/informer/cron.log 2>&1
```

## 订阅管理命令

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

Agent 的接口地址、密钥与模型是全局配置，写在 `informer.json` 的 `agent` 节（密钥除外），详见 [README 配置文件](README.md#三配置文件)。
前置条件：本机已安装对应命令行 Agent（例如 `claude --version` 能正常输出）。
