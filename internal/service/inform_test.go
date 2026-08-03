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
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/ding"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/lark"
	"github.com/vogo/informer/internal/service"
	"github.com/vogo/informer/internal/soup"
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

// frozenServer never answers a request until the test ends, the shape of a
// webhook that accepted the connection and then hung.
func frozenServer(t *testing.T) *httptest.Server {
	t.Helper()

	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	// cleanups run LIFO: release the frozen handler first, so server.Close
	// does not wait on a request that never answers.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	return server
}

// freezeDailySoup points the daily sentence at a frozen endpoint behind a short
// deadline, so the run under test never depends on the public endpoint and the
// soup step degrades in milliseconds instead of minutes.
func freezeDailySoup(t *testing.T, url string) {
	t.Helper()

	restoreURL := soup.SetURLForTest(url)
	restoreClient := soup.SetClientForTest(&http.Client{Timeout: 100 * time.Millisecond})

	t.Cleanup(restoreURL)
	t.Cleanup(restoreClient)
}

// TestTriggerInformFailsWhenTheLarkWebhookFreezes covers the stuck lock root
// cause end to end: a lark webhook that hangs must fail the whole run within
// the client deadline, keep the daily file, and leave the articles unrecorded
// so the scheduler retries the day on its next tick.
func TestTriggerInformFailsWhenTheLarkWebhookFreezes(t *testing.T) {
	server := newContentServer(t)
	frozen := frozenServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(feedSource(server)))
	freezeDailySoup(t, frozen.URL)

	restore := lark.SetClientForTest(&http.Client{Timeout: 100 * time.Millisecond})
	defer restore()

	start := time.Now()
	result, err := svc.TriggerInform(frozen.URL + "/feishu.cn/hang")
	elapsed := time.Since(start)

	require.Error(t, err, "a frozen webhook must fail the run")
	require.NotNil(t, result)
	assert.False(t, result.Notified)
	assert.Less(t, elapsed, 30*time.Second, "the client deadline must bound a frozen webhook")
	assert.FileExists(t, result.ContentFilePath, "the daily file survives a failed delivery")

	for _, article := range allArticles(t, svc) {
		assert.Nil(t, article.InformedAt, "a failed delivery must not be recorded as informed")
	}

	// the failure is a terminal result, not a wedged state: with a healthy
	// webhook, the very next run succeeds instead of staying blocked.
	result, err = svc.TriggerInform(server.URL + "/feishu.cn/ok")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Notified)
}

// TestTriggerInformFailsWhenTheDingWebhookFreezes repeats the frozen webhook
// proof for the dingtalk channel.
func TestTriggerInformFailsWhenTheDingWebhookFreezes(t *testing.T) {
	server := newContentServer(t)
	frozen := frozenServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(feedSource(server)))
	freezeDailySoup(t, frozen.URL)

	restore := ding.SetClientForTest(&http.Client{Timeout: 100 * time.Millisecond})
	defer restore()

	start := time.Now()
	result, err := svc.TriggerInform(frozen.URL + "/dingtalk.com/hang")
	elapsed := time.Since(start)

	require.Error(t, err, "a frozen webhook must fail the run")
	require.NotNil(t, result)
	assert.False(t, result.Notified)
	assert.Less(t, elapsed, 30*time.Second, "the client deadline must bound a frozen webhook")
	assert.FileExists(t, result.ContentFilePath, "the daily file survives a failed delivery")

	for _, article := range allArticles(t, svc) {
		assert.Nil(t, article.InformedAt, "a failed delivery must not be recorded as informed")
	}
}

// TestTriggerInformSucceedsWhenTheDailySoupFreezes proves the soup deadline is
// a degradation only: the fetch, the daily file and the bot delivery all
// survive a frozen daily sentence endpoint.
func TestTriggerInformSucceedsWhenTheDailySoupFreezes(t *testing.T) {
	server := newContentServer(t)
	frozen := frozenServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(feedSource(server)))
	freezeDailySoup(t, frozen.URL)

	result, err := svc.TriggerInform(server.URL + "/feishu.cn/ok")
	require.NoError(t, err, "a frozen daily sentence must not fail the run")
	require.NotNil(t, result)
	assert.True(t, result.Notified)
	assert.Len(t, result.Articles, 2)
	assert.FileExists(t, result.ContentFilePath)
}
