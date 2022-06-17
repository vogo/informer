operate 
```bash
./informer feed add github_golang_daily https://kaifa.baidu.com/rest/v1/home/github\?optionLanguage\=go\&optionSince\=DAILY
./informer feed list
./informer feed view 2
./informer feed update 2 weight 50
./informer feed update 2 max_fetch_num 5
./informer feed update 2 max_fetch_num 2
./informer feed update 2 regex ",\"url\":\"([^\"]+)\",\"title\":\"([^\"]+)\","
./informer feed update 2 title_group 2
./informer feed update 2 url_group 1
./informer feed view 2

# id:     2
# title:  github_golang_daily
# url:    https://kaifa.baidu.com/rest/v1/home/github?optionLanguage=go&optionSince=DAILY
# weight: 50
# max_fetch_num:  2
# regex:  ,"url":"([^"]+)","title":"([^"]+)",
# title_group:        2
# url_group: 1

./informer feed parse 2
```