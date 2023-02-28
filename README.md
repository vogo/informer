# 信息通知-日期、鸡汤、feed文章推荐

## 安装 informer
```bash
GOBIN=$(pwd) go install github.com/vogo/informer@master
```

## 创建配置文件

参考[配置范例](informer.json)

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

# 测试抓取
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

## 配置机器人

配置钉钉或飞书机器人, 关键字审核模式，得到机器人地址 https://oapi.dingtalk.com/robot/send?access_token=xxxxx。

## 配置 linux crontab 定时任务

每天早上10点发到钉钉：
```
00 10 * * * /root/informer/informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx >> /root/informer/cron.log"
```

或每天早上10点发到飞书：
```
00 10 * * * /root/informer/informer "https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxxx >> /root/informer/cron.log"
```