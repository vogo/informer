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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateAndFilterFeeds(t *testing.T) {
	feedConfig := &FeedConfig{
		MaxInformFeedSize: 10,
		FeedExpireDays:    15,
		SameSiteMaxCount:  2,
		Feeds: []*FeedSource{
			{
				URL:    "http://blog.sciencenet.cn/rss.php?uid=117333",
				Weight: 100,
			},
		},
	}

	feedData := make(map[string]*FeedDetail)

	articles := updateAndFilterFeeds(feedConfig, feedData)
	if len(articles) == 0 {
		t.Error("parse feed article failed")
	} else {
		articlesInfo, _ := json.Marshal(articles)
		t.Log(string(articlesInfo))
	}
}

func TestGetHostFromUrl(t *testing.T) {
	assert.Equal(t, "www.blog.com", getHostFromUrl("http://www.blog.com/page.html"))
}
