package informer

import (
	"encoding/json"
	"testing"
)

func TestUpdateAndFilterFeeds(t *testing.T) {
	feedConfigs := []*FeedConfig{
		{
			URL:    "http://blog.sciencenet.cn/rss.php?uid=117333",
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
