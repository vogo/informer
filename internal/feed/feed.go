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

package feed

import (
	"io"
	"math"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/vogo/logger"
)

const (
	oneDaySeconds = int64(24 * time.Hour / time.Second)
)

func AddFeeds(buf io.StringWriter, config *Config) {
	articles := UpdateAndFilterFeeds(config)
	if len(articles) > 0 {
		_, _ = buf.WriteString("文章推荐:\n")

		for _, a := range articles {
			_, _ = buf.WriteString("- " + a.Title + ", " + a.URL + "\n")
		}
	}
}

func UpdateAndFilterFeeds(config *Config) []*Article {
	now := time.Now()
	nowTime := now.Unix()
	expireTime := now.Add(time.Hour * 24 * time.Duration(-config.FeedExpireDays)).Unix()

	minWeight := int64(math.MaxInt64)
	maxWeight := int64(0)

	var sources []*Source

	feedDataDB.Model(&Source{}).Order("id").Find(&sources)
	sources = append(sources, config.Feeds...)

	for _, source := range sources {
		addFeed(config, source, expireTime)

		if minWeight > source.Weight {
			minWeight = source.Weight
		}

		if maxWeight < source.Weight {
			maxWeight = source.Weight
		}
	}

	dayIntervalWeight := (maxWeight - minWeight) / int64(config.FeedExpireDays)

	updateExistsScore(nowTime, dayIntervalWeight)

	return sortAndChoseArticles(config)
}

func updateExistsScore(nowTime, dayIntervalWeight int64) {
	var articleList []*Article

	feedDataDB.Model(&Article{}).Where("informed=?", false).Find(&articleList)

	for _, article := range articleList {
		// adjust timestamp
		if article.Timestamp == 0 {
			article.Timestamp = nowTime
		}

		pastDays := (nowTime - article.Timestamp) / oneDaySeconds
		newScore := article.Weight - pastDays*dayIntervalWeight

		feedDataDB.Model(article).Update("score", newScore)
	}
}

const articleChoseSizeMultiple = 4

func sortAndChoseArticles(config *Config) []*Article {
	var articleList []*Article

	feedDataDB.Model(&Article{}).
		Where("informed=?", false).
		Order("score desc").
		Order("id desc").
		Limit(config.MaxInformFeedSize * articleChoseSizeMultiple).
		Find(&articleList)

	informArticles := choseArticle(articleList, config)
	for _, a := range informArticles {
		feedDataDB.Model(a).Update("informed", true)
	}

	return informArticles
}

func choseArticle(list []*Article, config *Config) []*Article {
	// nolint:prealloc //ignore this
	var articles []*Article

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

func addFeed(config *Config, source *Source, expireTime int64) {
	logger.Info("parse feed: ", source.URL)

	fp := gofeed.NewParser()

	feed, err := fp.ParseURL(source.URL)
	if err != nil {
		logger.Infof("parse feed url error! url: %s, error: %v", source.URL, err)

		return
	}

	count := 0

	for _, item := range feed.Items {
		addFeedItem(source, expireTime, item)

		count++

		if source.MaxFetchNum > 0 {
			if count >= source.MaxFetchNum {
				break
			}
		} else if config.MaxFetchNum > 0 && count >= config.MaxFetchNum {
			break
		}
	}
}

func addFeedItem(source *Source, expireTime int64, item *gofeed.Item) {
	urlAddr, ok := FormatURL(item.Link)
	if !ok {
		return
	}

	var existCount int64

	feedDataDB.Model(&Article{}).Where("url=?", urlAddr).Count(&existCount)

	if existCount > 0 {
		logger.Warnf("add feed: %s, %s", item.Title, item.Link)

		return
	}

	logger.Infof("add feed: %s, %s", item.Title, item.Link)

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

	article := &Article{
		Title:     item.Title,
		Timestamp: timestamp,
		Weight:    source.Weight,
		Informed:  false,
		URL:       urlAddr,
	}

	feedDataDB.Save(article)
}
