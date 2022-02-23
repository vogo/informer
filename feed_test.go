package informer

import (
	"encoding/json"
	"testing"
)

func TestUpdateAndFilterFeeds(t *testing.T) {
	feedConfig := &FeedConfig{
		MaxInformFeedSize: 10,
		FeedExpireDays:    15,
		SameSiteMaxCount:  2,
		Feeds: []*FeedSource{
			{
				URL:    "http://blog.sciencenet.cn/rss.php?uid=117333",
				Weight: 100,
			},
		},
	}

	feedData := make(map[string]*FeedDetail)

	articles := updateAndFilterFeeds(feedConfig, feedData)
	if len(articles) == 0 {
		t.Error("parse feed article failed")
	} else {
		articlesInfo, _ := json.Marshal(articles)
		t.Log(string(articlesInfo))
	}
}
