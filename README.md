# 信息通知-日期、鸡汤、feed文章推荐

## 安装 informer
```bash
GOBIN=$(pwd) go install github.com/vogo/informer@master
```

命令行入口为 `cmd/informer`（根包入口为保持上面的安装方式而保留，两者行为完全一致）。
CLI 可以在 `CGO_ENABLED=0` 下构建。

## 桌面版 informer-ui

`cmd/informer-ui` 是 Wails v2 + Vue 3 + Naive UI 的桌面入口，与 CLI 共用同一数据目录和 Service 层。
桌面版需要 CGO（原生 WebView 与 mattn/go-sqlite3）。窗口顶部分为四个 Tab：

| Tab | 能做什么 |
| --- | --- |
| **订阅** | 左侧分类树：新增 / 编辑 / 删除分类，用整数「排序值」决定顺序（越小越靠前，相同排序值按分类 ID 排）。右侧按分类筛选的订阅卡片：展示解析类型、抓取状态、所属分类、错误信息，卡片右上角的开关直接启停订阅，底部可「测试抓取」（真实抓取，但不写库、不改订阅状态）、编辑、删除。 |
| **日报** | 左侧按「年 → 月 → 日」折叠的日期列表，点击某天在右侧全宽渲染当天的 Markdown。单日内容一次性加载，不分页。 |
| **文章库** | 已入库文章的游标翻页列表，可按分类、订阅、关键字筛选，显示通知时间；标题点击后在系统浏览器中打开。 |
| **设置** | 读写 `informer.json` 的抓取与推荐参数、配置机器人地址（写入独立的敏感文件）、执行「重建历史索引」。 |

几条与数据安全相关的实现约定：

- 日报 Markdown 以关闭原始 HTML 的方式渲染，并在注入 DOM 前做一次消毒；报告里的链接一律走系统浏览器打开，不会让 WebView 自身跳转。
- 日期只接受 `2006-01-02` 这一种写法并由后端校验，界面无法读到数据目录以外的文件。
- 文章库翻页用文章 ID 作游标而不是 offset，定时任务在翻页期间插入新文章也不会造成重复或漏项；切换筛选条件会回到第一页。

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

informer 的所有数据（`informer.json`、`informer.secret.json`、`feed.db`、`data/<年份>/<日期>.md`）都存放在**同一个数据主目录**中：

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

参考[配置范例](informer.json)，把它放到数据主目录下并命名为 `informer.json`。也可以直接在桌面版的「设置」Tab 里填写并保存，
两者写的是同一个文件，CLI 与 crontab 读取的也是它。

配置文件保持「人类可读的整份 JSON」，不入库、不做版本历史，写入时遵守下面几条约定：

- **保留未知字段**：保存只替换 `feed` 这一节，文件里其它顶层字段（包括这个版本还不认识的字段）连同顺序一起原样保留。
- **原子替换**：先写同目录下的临时文件再 `rename` 覆盖，所以并发运行的 crontab 读到的要么是旧文件、要么是新文件，不会读到写了一半的内容。
- **跨进程写锁**：写入前会创建 `informer.json.lock`，两个「读—改—写」不会互相覆盖造成丢失更新。锁有等待上限，
  等不到会直接报错而不是一直卡住；进程被杀留下的陈旧锁会在明显过期后被自动接管。
- **校验**：`feed_expire_days`、`max_inform_feed_size`、`same_site_max_count` 必须大于 0（为 0 会让推荐算法选不出任何文章），
  `max_fetch_num` 填 0 表示不做全局限制。超出合理范围的值会被拒绝，且被拒绝的保存不会改动文件。

### 敏感配置文件

机器人地址（webhook）不写进 `informer.json`，而是单独存放在数据主目录下的 `informer.secret.json`：

```json
{
  "webhook": "https://oapi.dingtalk.com/robot/send?access_token=xxxxx"
}
```

- 该文件无论新建还是改写，权限都会被设置并校验为 **0600**；权限无法落实时保存直接失败，不会以不安全的状态写下去
  （Windows 不支持 Unix 权限位，这一步在 Windows 上无法校验）。
- 桌面版「设置」页只显示「是否已配置」和一段打码后的前缀，完整地址不会回传到界面。
- 命令行显式传入的地址**优先**，因此现有 crontab 的行为完全不变；不传地址时才使用这个文件里保存的 webhook。

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

## 重建历史索引

早期版本不记录「文章实际在哪一天被推送」，这些老文章的 `informed_at` 是空的。桌面版「设置 → 重建历史索引」可以从已经生成的
日报 Markdown 里把这部分信息补回来：

- **判定依据只有链接**：从日报里提取文章链接，与库中文章的 URL 做**精确**匹配。只有「恰好匹配到一条、且该文章的通知时间为空」
  才会补值，补的是该日报**当天的本地 00:00**（精度到天，因为文件名只能证明到天）。
- **不覆盖、不编造**：已经有通知时间的文章原样保留；库里找不到的链接、匹配到多条文章的链接，一律跳过并保持为空。
- **可重复执行**：结果幂等，第二次运行只会把第一次补好的记录报告为「已有时间」。
- 执行后会给出扫描天数、链接数，以及成功 / 跳过（分「已有时间」「库中无此链接」「匹配到多条」）/ 失败的数量。

明确的非目标：不做拖拽排序、不做订阅批量导入导出、不做全文搜索、不用启发式手段猜测历史数据，也不承诺覆盖 100% 的历史文章；
配置不迁移到数据库、不提供配置的版本历史或回滚，也不支持多用户 / 多环境配置。

## 配置机器人

配置钉钉或飞书机器人, 关键字审核模式，得到机器人地址 https://oapi.dingtalk.com/robot/send?access_token=xxxxx。
地址可以在命令行显式传入（见下方 crontab 示例），也可以在桌面版「设置」页保存到 [敏感配置文件](#敏感配置文件)。

## 配置 linux crontab 定时任务

每天早上10点发到钉钉：
```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx" >> /root/informer/cron.log 2>&1
```

或每天早上10点发到飞书：
```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer "https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxxx" >> /root/informer/cron.log 2>&1
```

机器人地址已经保存在 `informer.secret.json` 时，可以不带地址参数：

```
00 10 * * * INFORMER_HOME=/root/.informer /root/informer/informer >> /root/informer/cron.log 2>&1
```

不写 `INFORMER_HOME` 时使用 `~/.informer`；数据目录与迁移方式见上文[数据目录](#数据目录)。