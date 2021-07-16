# 自动点餐消息发送钉钉

## 配置钉钉机器人

配置钉钉机器人, 关键字审核模式，得到钉钉机器人地址 https://oapi.dingtalk.com/robot/send?access_token=xxxxx。

## 安装 foodorder
```bash
GOBIN=$(pwd) go get -u github.com/wongoo/foodorder@master
```

## 创建配置文件

参考[配置范例](foodorder.json)

## 配置 linux crontab 定时任务
```bash
# 每天早上10点发
00 10 * * * /root/foodorder/foodorder "https://oapi.dingtalk.com/robot/send?access_token=xxxxx >> /root/foodorder/cron.log"
```