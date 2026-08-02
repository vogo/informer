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

package service_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// allArticles reads every stored article, ordered by id.
func allArticles(t *testing.T, svc *service.Service) []*feed.Article {
	t.Helper()

	page, err := svc.ListArticles(service.ArticleQuery{}, service.PageRequest{Limit: service.MaxPageLimit})
	require.NoError(t, err)

	return page.Items
}

func TestTriggerInformFetchesOnlyEnabledSources(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	enabled := feedSource(server)
	require.NoError(t, svc.CreateSource(enabled))

	disabled := jsonSource(server)
	require.NoError(t, svc.CreateSource(disabled))
	require.NoError(t, svc.SetSourceEnabled(disabled.ID, false))

	result, err := svc.TriggerInform("")
	require.NoError(t, err)
	require.NotNil(t, result)

	articles := allArticles(t, svc)
	require.Len(t, articles, 2, "only the enabled source is fetched")

	for _, article := range articles {
		assert.Equal(t, enabled.ID, article.SourceID)
	}

	page, err := svc.ListArticles(service.ArticleQuery{SourceID: disabled.ID}, service.PageRequest{})
	require.NoError(t, err)
	assert.Empty(t, page.Items, "a disabled source contributes nothing")
}

// TestTriggerInformStampsFetchedAt covers all three parsers: every article that is
// stored for the first time carries a fetch time.
func TestTriggerInformStampsFetchedAt(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(feedSource(server)))
	require.NoError(t, svc.CreateSource(regexSource(server)))
	require.NoError(t, svc.CreateSource(jsonSource(server)))

	_, err := svc.TriggerInform("")
	require.NoError(t, err)

	articles := allArticles(t, svc)
	require.Len(t, articles, 6, "each of the three sources contributes two articles")

	for _, article := range articles {
		require.NotNil(t, article.FetchedAt, "article %q must carry a fetch time", article.Title)
		assert.Positive(t, *article.FetchedAt)
	}
}

// TestTriggerInformKeepsFirstFetchedAt proves a repeated url never rewrites the
// moment the article was first seen.
func TestTriggerInformKeepsFirstFetchedAt(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(feedSource(server)))

	_, err := svc.TriggerInform("")
	require.NoError(t, err)

	first := allArticles(t, svc)
	require.Len(t, first, 2)

	firstFetchedAt := map[int64]int64{}
	for _, article := range first {
		require.NotNil(t, article.FetchedAt)
		firstFetchedAt[article.ID] = *article.FetchedAt
	}

	_, err = svc.TriggerInform("")
	require.NoError(t, err)

	second := allArticles(t, svc)
	require.Len(t, second, 2, "the same urls are not stored twice")

	for _, article := range second {
		require.NotNil(t, article.FetchedAt)
		assert.Equal(t, firstFetchedAt[article.ID], *article.FetchedAt,
			"the first fetch time of %q must survive a later run", article.Title)
	}
}

func TestTriggerInformRecordsInformedAtOnSuccess(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(feedSource(server)))

	result, err := svc.TriggerInform(server.URL + "/feishu.cn/ok")
	require.NoError(t, err)
	require.True(t, result.Notified, "the bot accepted the message")
	require.Len(t, result.Articles, 2)

	for _, article := range allArticles(t, svc) {
		assert.True(t, article.Informed)
		require.NotNil(t, article.InformedAt, "a delivered article carries an inform time")
		assert.Positive(t, *article.InformedAt)
	}

	// the daily file is written next to the database, inside the active data directory.
	_, err = os.Stat(result.ContentFilePath)
	require.NoError(t, err)
	assert.Contains(t, result.ContentFilePath, svc.HomeDir())
}

// TestTriggerInformLeavesInformedAtEmptyOnFailure is the honesty rule for delivery:
// a notification that failed never produces an inform timestamp.
func TestTriggerInformLeavesInformedAtEmptyOnFailure(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(feedSource(server)))

	result, err := svc.TriggerInform(server.URL + "/feishu.cn/fail")
	require.Error(t, err, "a rejected notification is reported to the caller")
	require.NotNil(t, result)
	assert.False(t, result.Notified)

	articles := allArticles(t, svc)
	require.Len(t, articles, 2)

	for _, article := range articles {
		assert.Nil(t, article.InformedAt, "a failed delivery must not be recorded as informed")
	}
}

func TestTriggerInformWithoutConfigFile(t *testing.T) {
	svc, err := service.New(t.TempDir())
	require.NoError(t, err)

	_, err = svc.TriggerInform("")
	require.ErrorContains(t, err, "read config file")
}
