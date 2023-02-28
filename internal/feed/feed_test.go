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

package feed_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vogo/informer/internal/feed"
)

func TestUpdateAndFilterFeeds(t *testing.T) {
	t.Parallel()

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	feed.InitFeedDB(exeDir)

	feedConfig := &feed.Config{
		MaxInformFeedSize: 10,
		FeedExpireDays:    15,
		SameSiteMaxCount:  2,
	}

	feed.AddSource("test", "http://blog.sciencenet.cn/rss.php?uid=117333")

	articles := feed.UpdateAndFilterFeeds(feedConfig)
	if len(articles) == 0 {
		t.Error("parse feed article failed")
	} else {
		articlesInfo, err := json.Marshal(articles)
		if err != nil {
			t.Error(err)
		}

		t.Log(string(articlesInfo))
	}
}

func TestGetHostFromUrl(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "www.blog.com", feed.GetHostFromURL("http://www.blog.com/page.html"))
}
