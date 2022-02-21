package informer

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"math"
	"path/filepath"
	"sort"
	"time"

	"github.com/mmcdole/gofeed"
)

const (
	feedDataFile      = "feed_data.json"
	maxInformFeedSize = 5
	feedExpireDays    = 7
	oneDaySeconds     = int64(24 * time.Hour / time.Second)
)

type FeedConfig struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Weight int64  `json:"weight"`
}

type FeedDetail struct {
	Title     string `json:"title"`
	Timestamp int64  `json:"timestamp"`
	Weight    int64  `json:"weight"`
	Informed  bool   `json:"informed"`
	score     int64
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
	now := time.Now()
	nowTime := now.Unix()
	expireTime := now.Add(time.Hour * 24 * -feedExpireDays).Unix()

	minWeight := int64(math.MaxInt64)
	maxWeight := int64(0)
	for _, config := range configs {
		addFeed(feedData, config, expireTime)

		if minWeight > config.Weight {
			minWeight = config.Weight
		}
		if maxWeight < config.Weight {
			maxWeight = config.Weight
		}
	}

	dayIntervalWeight := int64((maxWeight - minWeight) / feedExpireDays)

	var deleteList []string
	var articleList []*FeedArticle
	for url, detail := range feedData {
		// adjust timestamp
		if detail.Timestamp == 0 {
			detail.Timestamp = nowTime
		}

		if detail.Timestamp < expireTime {
			deleteList = append(deleteList, url)
			continue
		}
		if detail.Informed {
			continue
		}

		article := &FeedArticle{
			FeedDetail: *detail,
			URL:        url,
		}

		pastDays := (nowTime - article.Timestamp) / oneDaySeconds
		article.score = article.Weight - pastDays*dayIntervalWeight
		articleList = append(articleList, article)
	}

	for _, key := range deleteList {
		delete(feedData, key)
	}

	sort.Slice(articleList, func(i, j int) bool {
		a := articleList[i]
		b := articleList[j]

		return a.score < b.score
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

func addFeed(data map[string]*FeedDetail, config *FeedConfig, expireTime int64) {
	log.Println("parse feed: ", config.URL)

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(config.URL)
	if err != nil {
		log.Printf("parse feed url error! url: %s, error: %v", config.URL, err)
		return
	}

	for _, item := range feed.Items {
		addFeedItem(data, config, expireTime, item)
	}
}

func addFeedItem(data map[string]*FeedDetail, config *FeedConfig, expireTime int64, item *gofeed.Item) {
	url := item.Link
	if _, exists := data[url]; exists {
		return
	}

	now := time.Now()
	date := now

	if item.UpdatedParsed != nil {
		date = *item.UpdatedParsed
	} else if item.PublishedParsed != nil {
		date = *item.PublishedParsed
	}

	if date.After(now) {
		date = now
	}

	timestamp := date.Unix()
	if timestamp < expireTime {
		return
	}

	data[url] = &FeedDetail{
		Title:     item.Title,
		Timestamp: timestamp,
		Weight:    config.Weight,
		Informed:  false,
	}
}
