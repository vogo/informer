/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package informer

import (
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

const (
	feedDataFile  = "feed_data.json"
	oneDaySeconds = int64(24 * time.Hour / time.Second)
)

type FeedConfig struct {
	MaxInformFeedSize int           `json:"max_inform_feed_size"`
	FeedExpireDays    int           `json:"feed_expire_days"`
	SameSiteMaxCount  int           `json:"same_site_max_count"`
	Feeds             []*FeedSource `json:"feeds"`
}

type FeedSource struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Weight int64  `json:"weight"`
}

type FeedDetail struct {
	Title     string `json:"title"`
	Timestamp int64  `json:"timestamp"`
	Weight    int64  `json:"weight"`
	Informed  bool   `json:"informed"`

	//nolint:structcheck //ignore this
	score int64
}

type FeedArticle struct {
	FeedDetail
	URL string `json:"url"`
}

func addFeeds(buf io.StringWriter, config *FeedConfig, exeDir string) {
	feedDateFilePath := filepath.Join(exeDir, feedDataFile)

	dataFile, err := os.ReadFile(feedDateFilePath)
	if err != nil {
		log.Printf("read feed data error: %v", err)
	}

	feedData := make(map[string]*FeedDetail)

	_ = json.Unmarshal(dataFile, &feedData)

	articles := UpdateAndFilterFeeds(config, feedData)
	if len(articles) > 0 {
		_, _ = buf.WriteString("文章推荐:\n")

		for _, a := range articles {
			_, _ = buf.WriteString("- " + a.Title + ", " + a.URL + "\n")
		}
	}

	if b, jsonErr := json.Marshal(feedData); jsonErr == nil {
		_ = os.WriteFile(feedDateFilePath, b, defaultDataFilePermission)
	}
}

func UpdateAndFilterFeeds(config *FeedConfig, feedData map[string]*FeedDetail) []*FeedArticle {
	now := time.Now()
	nowTime := now.Unix()
	expireTime := now.Add(time.Hour * 24 * time.Duration(-config.FeedExpireDays)).Unix()

	minWeight := int64(math.MaxInt64)
	maxWeight := int64(0)

	for _, source := range config.Feeds {
		addFeed(feedData, source, expireTime)

		if minWeight > source.Weight {
			minWeight = source.Weight
		}

		if maxWeight < source.Weight {
			maxWeight = source.Weight
		}
	}

	dayIntervalWeight := (maxWeight - minWeight) / int64(config.FeedExpireDays)

	articleList := filterArticles(feedData, nowTime, expireTime, dayIntervalWeight)

	return sortAndChoseArticles(config, feedData, articleList)
}

func filterArticles(feedData map[string]*FeedDetail, nowTime, expireTime, dayIntervalWeight int64) []*FeedArticle {
	var deleteList []string

	// nolint:prealloc //ignore this
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

	return articleList
}

func sortAndChoseArticles(config *FeedConfig, feedData map[string]*FeedDetail, articleList []*FeedArticle) []*FeedArticle {
	// order by score desc
	sort.Slice(articleList, func(i, j int) bool {
		a := articleList[i]
		b := articleList[j]

		return a.score > b.score
	})

	informArticles := choseArticle(articleList, config)
	for _, a := range informArticles {
		feedData[a.URL].Informed = true
	}

	return informArticles
}

func choseArticle(list []*FeedArticle, config *FeedConfig) []*FeedArticle {
	// nolint:prealloc //ignore this
	var articles []*FeedArticle

	previousArticleHost := ""
	sameSiteArticleCount := 0

	for _, article := range list {
		host := GetHostFromURL(article.URL)

		if previousArticleHost == host {
			if sameSiteArticleCount >= config.SameSiteMaxCount {
				continue
			}
			sameSiteArticleCount++
		} else {
			previousArticleHost = host
			sameSiteArticleCount = 1
		}

		articles = append(articles, article)

		if len(articles) >= config.MaxInformFeedSize {
			break
		}
	}

	return articles
}

// GetHostFromURL get host from url,
// host is www.blog.com if url is http://www.blog.com/page.html.
func GetHostFromURL(host string) string {
	hostIndex := strings.Index(host, "//")
	if hostIndex > 0 {
		host = host[hostIndex+2:]
	}

	hostIndex = strings.Index(host, "/")
	if hostIndex > 0 {
		host = host[:hostIndex]
	}

	return host
}

func addFeed(data map[string]*FeedDetail, config *FeedSource, expireTime int64) {
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

func addFeedItem(data map[string]*FeedDetail, config *FeedSource, expireTime int64, item *gofeed.Item) {
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
