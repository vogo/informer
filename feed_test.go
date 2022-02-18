package informer

import (
	"encoding/json"
	"testing"
)

func TestUpdateAndFilterFeeds(t *testing.T) {
	feedConfigs := []*FeedConfig{
		{
			Title:  "阮一峰的网络日志",
			URL:    "http://www.ruanyifeng.com/blog/atom.xml",
			Weight: 100,
		},
	}

	feedData := make(map[string]*FeedDetail)

	articles := updateAndFilterFeeds(feedConfigs, feedData)
	if len(articles) == 0 {
		t.Error("parse feed article failed")
	} else {
		articlesInfo, _ := json.Marshal(articles)
		t.Log(string(articlesInfo))
	}
}
