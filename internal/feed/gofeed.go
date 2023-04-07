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
)

// ParseGoFeed parse feed.
func ParseGoFeed(source *Source) (*gofeed.Feed, error) {
	fp := gofeed.NewParser()

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

		return
	}

	count := 0

	// sort feed items.
	sort.Slice(feed.Items, func(i, j int) bool {
		if feed.Items[i].PublishedParsed != nil && feed.Items[j].PublishedParsed != nil {
			return feed.Items[i].PublishedParsed.After(*feed.Items[j].PublishedParsed)
		}

		if feed.Items[i].UpdatedParsed != nil && feed.Items[j].UpdatedParsed != nil {
			return feed.Items[i].UpdatedParsed.After(*feed.Items[j].UpdatedParsed)
		}

		if feed.Items[i].Published == "" {
			return feed.Items[i].Published > feed.Items[j].Published
		}

		return feed.Items[i].Title > feed.Items[j].Title
	})

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
	urlAddr, ok := FormatURL(item.Link)
	if !ok {
		return
	}

	if source.Redirect {
		urlAddr = vurl.RedirectURL(urlAddr)
	}

	if isFeedURLExists(urlAddr) {
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
