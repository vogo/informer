# 自动点餐消息发送钉钉

## 配置机器人

配置钉钉或飞书机器人, 关键字审核模式，得到机器人地址 https://oapi.dingtalk.com/robot/send?access_token=xxxxx。

## 安装 informer
```bash
GOBIN=$(pwd) go install github.com/vogo/informer@master
```

## 创建配置文件

参考[配置范例](informer.json)

## 配置 linux crontab 定时任务

每天早上10点发到钉钉：
```
00 10 * * * /root/informer/informer "https://oapi.dingtalk.com/robot/send?access_token=xxxxx >> /root/informer/cron.log"
```

或每天早上10点发到飞书：
```
00 10 * * * /root/informer/informer "https://open.feishu.cn/open-apis/bot/v2/hook/xxxxxxxxx >> /root/informer/cron.log"
```