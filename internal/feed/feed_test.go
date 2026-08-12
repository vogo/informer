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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/feed"
)

// TestUpdateAndFilterFeeds fetches a real feed, so it is skipped in short mode.
// It shares the package level database handle, so it never runs in parallel.
func TestUpdateAndFilterFeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}

	db, err := feed.InitFeedDB(t.TempDir())
	require.NoError(t, err)

	feedConfig := &feed.Config{
		MaxInformFeedSize: 10,
		FeedExpireDays:    15,
		SameSiteMaxCount:  2,
	}

	require.NoError(t, db.Create(&feed.Source{
		Title:      "test",
		URL:        "http://blog.sciencenet.cn/rss.php?uid=117333",
		CategoryID: feed.DefaultCategoryID,
		Enabled:    true,
	}).Error)

	articles := feed.UpdateAndFilterFeeds(feedConfig, nil)
	if len(articles) == 0 {
		t.Error("parse feed article failed")
	} else {
		articlesInfo, marshalErr := json.Marshal(articles)
		if marshalErr != nil {
			t.Error(marshalErr)
		}

		t.Log(string(articlesInfo))
	}
}

func TestGetHostFromUrl(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "www.blog.com", feed.GetHostFromURL("http://www.blog.com/page.html"))
}
