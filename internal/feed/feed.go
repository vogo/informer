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
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/agent"
)

const (
	oneDaySeconds = int64(24 * time.Hour / time.Second)
)

// AddFeeds writes the recommended articles into buf and returns them so that the
// caller can record the delivery once a notification actually succeeded.
func AddFeeds(buf io.StringWriter, config *Config, agentConfig *agent.Config) []*Article {
	articles := UpdateAndFilterFeeds(config, agentConfig)
	if len(articles) > 0 {
		_, _ = buf.WriteString("文章推荐:\n")

		for _, a := range articles {
			_, _ = buf.WriteString("- " + a.Title + ", " + a.URL + "\n")
		}
	}

	return articles
}

func UpdateAndFilterFeeds(config *Config, agentConfig *agent.Config) []*Article {
	now := time.Now()
	nowTime := now.Unix()
	expireTime := now.Add(time.Hour * 24 * time.Duration(-config.FeedExpireDays)).Unix()

	minWeight := int64(math.MaxInt64)
	maxWeight := int64(0)

	var sources []*Source

	// only enabled sources take part in a real fetch; Preview is not restricted by Enabled.
	feedDataDB.Model(&Source{}).Where("enabled = ?", true).Order("id").Find(&sources)

	for _, source := range sources {
		switch source.ResolveParseType() {
		case ParseTypeJSON:
			jsonParseFeed(config, source, expireTime)
		case ParseTypeRegex:
			regexParseFeed(config, source, expireTime)
		case ParseTypeAgent:
			agentParseFeed(config, source, agentConfig, expireTime)
		default:
			addGoFeed(config, source, expireTime)
		}

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
	//nolint:prealloc //ignore this
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

func isFeedURLExists(url string) bool {
	var existCount int64

	feedDataDB.Model(&Article{}).Where("url=?", url).Count(&existCount)

	if existCount > 0 {
		logger.Warnf("exists feed: %s", url)

		return true
	}

	return false
}

func saveParsedArticles(config *Config, source *Source, articles []*Article) {
	count := 0

	for _, a := range articles {
		if source.MaxFetchNum > 0 {
			if count >= source.MaxFetchNum {
				break
			}
		} else if config.MaxFetchNum > 0 && count >= config.MaxFetchNum {
			break
		}

		count++

		if isFeedURLExists(a.URL) {
			continue
		}

		saveNewArticle(a)
	}
}

// saveNewArticle persists a freshly fetched article and stamps its first fetch time.
// Duplicate urls never reach this function, so FetchedAt is never overwritten.
func saveNewArticle(article *Article) {
	if article.FetchedAt == nil {
		fetchedAt := time.Now().Unix()
		article.FetchedAt = &fetchedAt
	}

	feedDataDB.Save(article)
}

// MarkArticlesInformed records the moment a notification actually delivered the articles.
// It is called only after the notification step succeeded, so a failed delivery never
// produces an inform timestamp.
func MarkArticlesInformed(articles []*Article, informedAt int64) error {
	if len(articles) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.ID)
		a.Informed = true
		a.InformedAt = &informedAt
	}

	return feedDataDB.Model(&Article{}).Where("id IN ?", ids).
		Updates(map[string]any{"informed": true, "informed_at": informedAt}).Error
}

func updateSourceError(source *Source, err error) {
	source.Status = StatusError
	source.ErrorInfo = err.Error()
	feedDataDB.Save(source)
}

func updateSourceNormal(source *Source) {
	source.Status = StatusNormal
	source.ErrorInfo = ""
	feedDataDB.Save(source)
}
