# 信息通知-日期、鸡汤、feed文章推荐

## 安装 informer
```bash
GOBIN=$(pwd) go install github.com/vogo/informer@master
```

命令行入口为 `cmd/informer`（根包入口为保持上面的安装方式而保留，两者行为完全一致）。
CLI 可以在 `CGO_ENABLED=0` 下构建。

## 桌面版 informer-ui

`cmd/informer-ui` 是 Wails v2 + Vue 3 + Naive UI 的桌面入口，与 CLI 共用同一数据目录和 Service 层，
提供订阅的新增 / 编辑 / 删除与「测试抓取」预览（真实抓取，但不写库、不改订阅状态）。
桌面版需要 CGO（原生 WebView 与 mattn/go-sqlite3）。

本地开发（需要安装 [wails v2 CLI](https://wails.io/docs/gettingstarted/installation)、Node.js 与 C 编译器）：

```bash
cd cmd/informer-ui
wails dev      # 本地运行
wails build    # 出包，产物在 build/bin/
```

发布：打 `v*` 标签后由 `release.yml` 在 macOS / Windows / Linux 三个原生 runner 上构建，
并聚合发布到 GitHub Release（macOS 为未签名 universal dmg，Windows 为内置 WebView2 引导的
NSIS 安装包，Linux 为 x86_64 tar.gz 与 deb）。本阶段不做代码签名 / 公证。

## 数据目录

informer 的所有数据（`informer.json`、`feed.db`、`data/<年份>/<日期>.md`）都存放在**同一个数据主目录**中：

| `INFORMER_HOME` | 数据主目录 |
| --- | --- |
| 未设置或为空 | `~/.informer`（Windows 为 `%USERPROFILE%\.informer`） |
| 已设置 | 该环境变量指向的目录（相对路径会转成绝对路径） |

数据目录不再依赖可执行文件所在位置，因此从 Finder / 桌面快捷方式启动时（不继承 shell 环境变量）也能找到同一份数据。
如果 `INFORMER_HOME` 指向的目录无法创建或不可写，informer 会直接报错退出，**不会**退回到另一份数据继续跑。

### 从旧版本迁移

旧版本把数据放在可执行文件所在目录。首次启动新版本时，informer 会自动把旧目录里的应用数据**复制**（不是移动）到数据主目录：

- 复制的内容：`informer.json`、`feed.db`（含 `-wal` / `-shm`）、`feed_data.json`、以及整个 `data/` 目录，**目录层级保持不变**；可执行文件本身和目录下的其它无关文件不会被复制。
- **冲突时保留目标并跳过**：数据主目录中已存在的同名文件一律保留，不会被旧文件覆盖。
- 旧目录的文件**不会被删除或清理**，请自行确认无误后再处理。
- 复制成功后会在数据主目录写入 `.migrated` 标记，后续启动不再重复迁移；若中途失败，informer 会报错退出，已复制的文件保留，修复后重跑会跳过已存在的文件继续完成。

**建议先备份**再升级：

```bash
# 假设旧数据在 /root/informer
cp -a /root/informer /root/informer.backup.$(date +%Y%m%d)
```

### crontab 中设置 / 切换 INFORMER_HOME

crontab 不会加载 shell 的 profile，需要在命令里显式设置：

```bash
# 显式指定数据目录（推荐，路径确定，便于备份）
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx" >> /root/informer/cron.log 2>&1
```

不设置时使用 `~/.informer`；注意 crontab 中 `~` 取决于运行该任务的用户，显式写绝对路径更稳妥。

切换到另一个目录时，先把当前数据主目录整体复制过去，再修改 crontab 中的 `INFORMER_HOME`：

```bash
cp -a ~/.informer /data/informer-home
# 然后把 crontab 里的 INFORMER_HOME 改成 /data/informer-home
```

informer 不会自动同步多个数据主目录，也不支持同时使用多个 `INFORMER_HOME`。

## 创建配置文件

参考[配置范例](informer.json)，把它放到数据主目录下并命名为 `informer.json`。

## 通过命令添加订阅

订阅feed内容范例:
```bash
# 列出所有订阅地址
./informer feed list
# 添加订阅
./informer feed add "阮一峰blog" http://www.ruanyifeng.com/blog/atom.xml
# 设置文章排序权重
./informer feed update 1 weight 80
# 设置最大抓取文章数
./informer feed update 1 max_fetch_num 1
./informer feed view 1
# id:	1
# title:	阮一峰blog
# url:	http://www.ruanyifeng.com/blog/atom.xml
# c_url:
# weight:	80
# max_fetch_num:	1
# regex:
# title_exp:
# url_exp:
# redirect:	false
# parse_type:	feed
# category_id:	1
# enabled:	true

# 测试抓取（只解析并打印，不写库、不改订阅状态）
./informer feed parse 1
# 科技爱好者周刊（第 243 期）：与孔子 AI 聊天 : http://www.ruanyifeng.com/blog/2023/02/weekly-issue-243.html
# 科技爱好者周刊（第 242 期）：一次尴尬的服务器被黑 : http://www.ruanyifeng.com/blog/2023/02/weekly-issue-242.html
# 科技爱好者周刊（第 241 期）：中国的增长动力在内陆 : http://www.ruanyifeng.com/blog/2023/02/weekly-issue-241.html
```

订阅正则匹配范例:
```bash
# 添加订阅 https://www.julian.com/ 的文章, 非feed格式，需要正则匹配
./informer feed add "Julian Shapiro blog" https://www.julian.com/
./informer feed update 14 regex '<a href="([^"]+)" class="blog-post-link[^"]+"><div class="blog-post-link-text">([^<]+)</div>'
./informer feed update 14 title_exp '$2'
./informer feed update 14 url_exp 'https://www.julian.com$1'
./informer feed view 14
# id:	14
# title:	Julian Shapiro blog
# url:	https://www.julian.com/
# c_url:
# weight:	50
# max_fetch_num:	2
# regex:	<a href="([^"]+)" class="blog-post-link[^"]+"><div class="blog-post-link-text">([^<]+)</div>
# title_exp:	$2
# url_exp:	https://www.julian.com$1
# redirect:	false

# 测试抓取 
./informer feed parse 14
# 2023/02/01 17:14:39.716 INFO regex parse, link: https://www.julian.com/blog/armageddon, title: Armageddon
# 2023/02/01 17:14:39.716 INFO regex parse, link: https://www.julian.com/blog/life-planning, title: What to do with your life
# Armageddon : https://www.julian.com/blog/armageddon
# What to do with your life : https://www.julian.com/blog/life-planning
```

订阅分类与启停:
```bash
# 列出所有分类，格式为 id,名称,排序值；id=1 的「未分类」是内置分类，不可删除
./informer feed category
# 1,	未分类,	0

# 把订阅归到某个分类下
./informer feed update 1 category_id 2

# 显式指定解析方式，合法值为 feed / regex / json
# 留空时仍按旧规则推导：is_json 为真用 json，否则 regex 非空用 regex，其余用 feed
./informer feed update 1 parse_type regex

# 停用订阅：停用后不再参与定时抓取，但 feed parse 仍可正常测试
./informer feed update 1 enabled false
```

## 配置机器人

配置钉钉或飞书机器人, 关键字审核模式，得到机器人地址 https://oapi.dingtalk.com/robot/send?access_token=xxxxx。

## 配置 linux crontab 定时任务

每天早上10点发到钉钉：
```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx" >> /root/informer/cron.log 2>&1
```

或每天早上10点发到飞书：
```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer "https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxxx" >> /root/informer/cron.log 2>&1
```

不写 `INFORMER_HOME` 时使用 `~/.informer`；数据目录与迁移方式见上文[数据目录](#数据目录)。