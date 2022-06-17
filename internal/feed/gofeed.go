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
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/vogo/logger"
)

func addGoFeed(config *Config, source *Source, expireTime int64) {
	logger.Info("parse feed: ", source.URL)

	fp := gofeed.NewParser()

	feed, err := fp.ParseURL(source.URL)
	if err != nil {
		logger.Infof("parse feed url error! url: %s, error: %v", source.URL, err)

		return
	}

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
	urlAddr, ok := FormatURL(item.Link)
	if !ok {
		return
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
