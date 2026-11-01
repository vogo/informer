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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
)

// TestUpdateAndFilterFeeds fetches a local RSS fixture.
func TestUpdateAndFilterFeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprintf(w, `<rss version="2.0"><channel><title>Fixture</title>
<link>https://example.com</link><description>Test feed</description>
<item><title>Fixture article</title><link>https://example.com/article</link>
<guid>https://example.com/article</guid><pubDate>%s</pubDate></item>
</channel></rss>`, time.Now().UTC().Format(time.RFC1123Z))
	}))
	t.Cleanup(server.Close)

	db, err := feed.InitFeedDB(t.TempDir())
	require.NoError(t, err)

	feedConfig := &feed.Config{
		MaxInformFeedSize: 10,
		FeedExpireDays:    15,
		SameSiteMaxCount:  2,
	}

	require.NoError(t, db.Create(&feed.Source{
		Title:      "test",
		URL:        server.URL,
		CategoryID: feed.DefaultCategoryID,
		Enabled:    true,
	}).Error)

	articles := feed.NewStore(db).UpdateAndFilterFeeds(feedConfig, nil)
	require.Len(t, articles, 1)
	assert.Equal(t, "Fixture article", articles[0].Title)
	assert.Equal(t, "https://example.com/article", articles[0].URL)
}

func TestGetHostFromUrl(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "www.blog.com", feed.GetHostFromURL("http://www.blog.com/page.html"))
}
