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
	"fmt"
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
//
//nolint:gosmopolitan //daily report heading is Chinese by design.
func (s *Store) AddFeeds(buf io.StringWriter, config *Config, agentConfig *agent.Config) []*Article {
	articles := s.UpdateAndFilterFeeds(config, agentConfig)
	if len(articles) > 0 {
		_, _ = buf.WriteString("文章推荐:\n")

		for _, a := range articles {
			_, _ = buf.WriteString("- " + a.Title + ", " + a.URL + "\n")
		}
	}

	return articles
}

func (s *Store) UpdateAndFilterFeeds(config *Config, agentConfig *agent.Config) []*Article {
	now := time.Now()
	nowTime := now.Unix()
	expireTime := now.Add(time.Hour * 24 * time.Duration(-config.FeedExpireDays)).Unix()

	minWeight := int64(math.MaxInt64)
	maxWeight := int64(0)

	var sources []*Source

	// only enabled sources take part in a real fetch; Preview is not restricted by Enabled.
	s.db.Model(&Source{}).Where("enabled = ?", true).Order("id").Find(&sources)

	for _, source := range sources {
		switch source.ResolveParseType() {
		case ParseTypeJSON:
			s.jsonParseFeed(config, source, expireTime)
		case ParseTypeRegex:
			s.regexParseFeed(config, source, expireTime)
		case ParseTypeAgent:
			s.agentParseFeed(config, source, agentConfig, expireTime)
		default:
			s.addGoFeed(config, source, expireTime)
		}

		if minWeight > source.Weight {
			minWeight = source.Weight
		}

		if maxWeight < source.Weight {
			maxWeight = source.Weight
		}
	}

	dayIntervalWeight := (maxWeight - minWeight) / int64(config.FeedExpireDays)

	s.updateExistsScore(nowTime, dayIntervalWeight)

	return s.sortAndChoseArticles(config)
}

func (s *Store) updateExistsScore(nowTime, dayIntervalWeight int64) {
	var articleList []*Article

	s.db.Model(&Article{}).Where("informed=?", false).Find(&articleList)

	for _, article := range articleList {
		// adjust timestamp
		if article.Timestamp == 0 {
			article.Timestamp = nowTime
		}

		pastDays := (nowTime - article.Timestamp) / oneDaySeconds
		newScore := article.Weight - pastDays*dayIntervalWeight

		s.db.Model(article).Update("score", newScore)
	}
}

const articleChoseSizeMultiple = 4

func (s *Store) sortAndChoseArticles(config *Config) []*Article {
	articles, err := s.SelectArticles(config)
	if err != nil {
		logger.Warnf("select candidate articles: %v", err)
	}

	return articles
}

// SelectArticles reads existing candidates without fetching or updating any record.
func (s *Store) SelectArticles(config *Config) ([]*Article, error) {
	if config == nil {
		return []*Article{}, nil
	}
	var articleList []*Article

	result := s.db.Model(&Article{}).
		Where("informed=?", false).
		Order("score desc").
		Order("id desc").
		Limit(config.MaxInformFeedSize * articleChoseSizeMultiple).
		Find(&articleList)

	if result.Error != nil {
		return nil, fmt.Errorf("select articles: %w", result.Error)
	}

	return choseArticle(articleList, config), nil
}

func choseArticle(list []*Article, config *Config) []*Article {
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

func (s *Store) isFeedURLExists(url string) bool {
	var existCount int64

	s.db.Model(&Article{}).Where("url=?", url).Count(&existCount)

	if existCount > 0 {
		logger.Warnf("exists feed: %s", url)

		return true
	}

	return false
}

func (s *Store) saveParsedArticles(config *Config, source *Source, articles []*Article) {
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

		if s.isFeedURLExists(a.URL) {
			continue
		}

		s.saveNewArticle(a)
	}
}

// saveNewArticle persists a freshly fetched article and stamps its first fetch time.
// Duplicate urls never reach this function, so FetchedAt is never overwritten.
func (s *Store) saveNewArticle(article *Article) {
	if article.FetchedAt == nil {
		fetchedAt := time.Now().Unix()
		article.FetchedAt = &fetchedAt
	}

	s.db.Save(article)
}

// MarkArticlesInformed records the moment a notification actually delivered the articles.
// It is called only after the notification step succeeded, so a failed delivery never
// produces an inform timestamp.
func (s *Store) MarkArticlesInformed(articles []*Article, informedAt int64) error {
	if len(articles) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(articles))
	for _, a := range articles {
		ids = append(ids, a.ID)
		a.Informed = true
		a.InformedAt = &informedAt
	}

	return s.db.Model(&Article{}).Where("id IN ?", ids).
		Updates(map[string]any{"informed": true, "informed_at": informedAt}).Error
}

func (s *Store) updateSourceError(source *Source, err error) {
	source.Status = StatusError
	source.ErrorInfo = err.Error()
	s.db.Save(source)
}

func (s *Store) updateSourceNormal(source *Source) {
	source.Status = StatusNormal
	source.ErrorInfo = ""
	s.db.Save(source)
}
