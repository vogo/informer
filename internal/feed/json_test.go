package feed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJsonParseFeed(t *testing.T) {
	src := &Source{
		ID:            0,
		Title:         "test",
		URL:           "https://newsapi-hbr.caijingmobile.com/topic/detail?topic_id=3101&type=0&last_id=",
		CURL:          "",
		Weight:        50,
		MaxFetchNum:   100,
		Regex:         "",
		TitleExp:      "",
		URLExp:        "",
		Redirect:      false,
		Sort:          false,
		IsJSON:        true,
		JsonTitlePath: "data/article_list[]/article/title",
		JsonURLPath:   "data/article_list[]/article/share_url",
	}
	articles, err := JsonParseFeed(nil, src, time.Now().Unix())
	assert.Nil(t, err)
	assert.NotNil(t, articles)
	for _, a := range articles {
		t.Log(a)
	}
}
