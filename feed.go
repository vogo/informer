package informer

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

const (
	feedDataFile      = "feed_data.json"
	maxInformFeedSize = 5
)

type FeedConfig struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

type FeedDetail struct {
	Title    string `json:"title"`
	Date     string `json:"date"`
	Weight   int    `json:"weight"`
	Informed bool   `json:"informed"`
}

type FeedArticle struct {
	FeedDetail
	URL string `json:"url"`
}

func addFeeds(buf *bytes.Buffer, configs []*FeedConfig, exeDir string) {
	feedDateFilePath := filepath.Join(exeDir, feedDataFile)
	dataFile, err := ioutil.ReadFile(feedDateFilePath)
	if err != nil {
		log.Printf("read feed data error: %v", err)
	}

	feedData := make(map[string]*FeedDetail)

	_ = json.Unmarshal(dataFile, &feedData)

	articles := updateAndFilterFeeds(configs, feedData)
	if len(articles) > 0 {
		buf.WriteString("文章推荐:\n")
		for _, a := range articles {
			buf.WriteString("- " + a.Title + ", " + a.URL + "\n")
		}
	}

	if b, jsonErr := json.Marshal(feedData); jsonErr == nil {
		_ = ioutil.WriteFile(feedDateFilePath, b, 0660)
	}
}

func updateAndFilterFeeds(configs []*FeedConfig, feedData map[string]*FeedDetail) []*FeedArticle {
	sevenDaysBefore := time.Now().Add(time.Hour * 24 * -7).Format("2006-01-02")

	for _, config := range configs {
		addFeed(feedData, config, sevenDaysBefore)
	}

	var deleteList []string
	var articleList []*FeedArticle
	for url, detail := range feedData {
		if strings.Compare(detail.Date, sevenDaysBefore) < 0 {
			deleteList = append(deleteList, url)
			continue
		}
		if !detail.Informed {
			articleList = append(articleList, &FeedArticle{
				FeedDetail: *detail,
				URL:        url,
			})
		}
	}

	sort.Slice(articleList, func(i, j int) bool {
		a := articleList[i]
		b := articleList[j]

		if a.Weight != b.Weight {
			return a.Weight < b.Weight
		}

		return strings.Compare(a.Date, b.Date) < 0
	})

	size := maxInformFeedSize
	if len(articleList) < size {
		size = len(articleList)
	}

	informArticles := articleList[:size]
	for _, a := range informArticles {
		feedData[a.URL].Informed = true
	}
	return informArticles
}

func addFeed(data map[string]*FeedDetail, config *FeedConfig, sevenDaysBefore string) {
	log.Println("parse feed: ", config.URL)

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(config.URL)
	if err != nil {
		log.Printf("parse feed url error! url: %s, error: %v", config.URL, err)
		return
	}

	for _, item := range feed.Items {
		addFeedItem(data, config, sevenDaysBefore, item)
	}
}

func addFeedItem(data map[string]*FeedDetail, config *FeedConfig, sevenDaysBefore string, item *gofeed.Item) {
	url := item.Link
	if _, exists := data[url]; exists {
		return
	}

	var date string
	if item.Updated != "" {
		date = item.Updated
	} else if item.Published != "" {
		date = item.Published
	} else {
		date = time.Now().Format("2006-01-02")
	}

	if strings.Compare(date, sevenDaysBefore) < 0 {
		return
	}

	data[url] = &FeedDetail{
		Title:    item.Title,
		Date:     date,
		Weight:   config.Weight,
		Informed: false,
	}
}
