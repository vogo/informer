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
	"sort"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/vogo/logger"
	"github.com/vogo/vogo/vnet/vurl"

	"github.com/vogo/informer/internal/httpx"
)

// ParseGoFeed parse feed.
func ParseGoFeed(source *Source) (*gofeed.Feed, error) {
	fp := gofeed.NewParser()
	// bound the fetch with the shared 60s client: the parser default has no
	// timeout, so a source that accepts the connection but never answers would
	// otherwise park the inform run, and its lock, forever.
	fp.Client = httpx.HTTPClient

	feed, err := fp.ParseURL(source.URL)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	if source.Sort {
		// sort feed items.
		sort.Slice(feed.Items, func(i, j int) bool {
			// some published time is in the future, so we need to check it.
			if feed.Items[i].PublishedParsed != nil && feed.Items[j].PublishedParsed != nil &&
				feed.Items[i].PublishedParsed.Before(now) && feed.Items[j].PublishedParsed.Before(now) {
				return feed.Items[i].PublishedParsed.After(*feed.Items[j].PublishedParsed)
			}

			// some updated time is in the future, so we need to check it.
			if feed.Items[i].UpdatedParsed != nil && feed.Items[j].UpdatedParsed != nil &&
				feed.Items[i].UpdatedParsed.Before(now) && feed.Items[j].UpdatedParsed.Before(now) {
				return feed.Items[i].UpdatedParsed.After(*feed.Items[j].UpdatedParsed)
			}

			// the link most likely contains id which can used to sort.
			return feed.Items[i].Link > feed.Items[j].Link
		})
	}

	return feed, nil
}

func addGoFeed(config *Config, source *Source, expireTime int64) {
	logger.Info("parse feed: ", source.URL)

	feed, err := ParseGoFeed(source)
	if err != nil {
		logger.Warnf("parse feed url error! url: %s, error: %v", source.URL, err)

		updateSourceError(source, err)

		return
	}

	updateSourceNormal(source)

	count := 0

	for _, item := range feed.Items {
		addGoFeedItem(source, expireTime, item)

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

func addGoFeedItem(source *Source, expireTime int64, item *gofeed.Item) {
	article, ok := goFeedItemArticle(source, item)
	if !ok {
		return
	}

	if isFeedURLExists(article.URL) {
		return
	}

	logger.Infof("add feed: %s, %s", item.Title, item.Link)

	if article.Timestamp < expireTime {
		return
	}

	saveNewArticle(article)
}

// goFeedItemArticle converts a feed item into an article without touching the database.
func goFeedItemArticle(source *Source, item *gofeed.Item) (*Article, bool) {
	urlAddr, ok := FormatURL(item.Link)
	if !ok {
		return nil, false
	}

	if source.Redirect {
		urlAddr = vurl.RedirectURL(urlAddr)
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

	return &Article{
		Title:     item.Title,
		Timestamp: date.Unix(),
		Weight:    source.Weight,
		Informed:  false,
		URL:       urlAddr,
		SourceID:  source.ID,
	}, true
}

// GoFeedArticles parses the source as a feed and returns the candidate articles
// without reading or writing any persisted record.
func GoFeedArticles(source *Source) ([]*Article, error) {
	feedData, err := ParseGoFeed(source)
	if err != nil {
		return nil, err
	}

	//nolint:prealloc //ignore this.
	var articles []*Article

	for i, item := range feedData.Items {
		if source.MaxFetchNum > 0 && i >= source.MaxFetchNum {
			break
		}

		article, ok := goFeedItemArticle(source, item)
		if !ok {
			continue
		}

		articles = append(articles, article)
	}

	return articles, nil
}
