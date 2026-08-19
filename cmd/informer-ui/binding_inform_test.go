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

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockedBotPath is the bot endpoint the guard tests point the webhook at: it
// carries the lark host so the notification step routes to the real client, and
// the handler blocks there until the test releases it.
const blockedBotPath = "/feishu.cn/block"

// validFeedDTO is a feed section every validation rule accepts, the payload the
// trigger tests store so an inform run can start at all.
func validFeedDTO() *FeedConfigDTO {
	return &FeedConfigDTO{
		MaxInformFeedSize: 5,
		FeedExpireDays:    30,
		SameSiteMaxCount:  2,
		MaxFetchNum:       0,
	}
}

// newBotServer serves bot endpoints that carry the lark host in their path, so the
// notification step routes to the real client code while staying inside the test.
func newBotServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/feishu.cn/ok", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0}`))
	})
	mux.HandleFunc("/feishu.cn/fail", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bot rejected the message", http.StatusInternalServerError)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func TestTriggerNowReportsAMissingConfigFile(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	// no informer.json yet: the failure arrives as data, not as a binding error.
	result, err := app.TriggerNow()
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.False(t, result.Notified)
	assert.Zero(t, result.ArticleCount)
	assert.Empty(t, result.ContentFilePath)
	assert.Contains(t, result.ErrorInfo, "read config file")
}

func TestTriggerNowWritesTheDailyFileAndDelivers(t *testing.T) {
	t.Parallel()

	server := newBotServer(t)
	app := newTestApp(t)

	require.NoError(t, app.SaveConfig(validFeedDTO()))
	require.NoError(t, app.SaveWebhook(server.URL+"/feishu.cn/ok"))

	result, err := app.TriggerNow()
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Empty(t, result.ErrorInfo)
	assert.True(t, result.Notified)

	// the daily markdown is the run's durable artifact, inside the data directory.
	require.NotEmpty(t, result.ContentFilePath)
	assert.FileExists(t, result.ContentFilePath)
	assert.Contains(t, result.ContentFilePath, app.homeDir)
}

func TestTriggerNowReportsADeliveryFailureAfterWriting(t *testing.T) {
	t.Parallel()

	server := newBotServer(t)
	app := newTestApp(t)

	require.NoError(t, app.SaveConfig(validFeedDTO()))
	require.NoError(t, app.SaveWebhook(server.URL+"/feishu.cn/fail"))

	// the bot refuses the message, but the daily file was already written and the
	// page must hear about both facts at once.
	result, err := app.TriggerNow()
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.False(t, result.Notified)
	assert.NotEmpty(t, result.ErrorInfo)

	require.NotEmpty(t, result.ContentFilePath)
	assert.FileExists(t, result.ContentFilePath)
}

func TestTriggerNowDeliversWithoutAWebhook(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	require.NoError(t, app.SaveConfig(validFeedDTO()))

	// no bot address configured: the run still succeeds, it only skips delivery.
	result, err := app.TriggerNow()
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.False(t, result.Notified)
	assert.Empty(t, result.ErrorInfo)
	assert.FileExists(t, result.ContentFilePath)
}

func TestTriggerNowRefusesWhileARunIsInFlight(t *testing.T) {
	t.Parallel()

	reached := make(chan struct{}, 1)
	release := make(chan struct{})

	// the bot endpoint blocks until released, so the first run provably holds the
	// process wide guard while the second one asks for it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != blockedBotPath {
			http.NotFound(w, r)

			return
		}

		select {
		case reached <- struct{}{}:
		default:
		}

		<-release

		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	app := newTestApp(t)
	require.NoError(t, app.SaveConfig(validFeedDTO()))
	require.NoError(t, app.SaveWebhook(server.URL+blockedBotPath))

	type outcome struct {
		result *InformResultDTO
		err    error
	}

	first := make(chan outcome, 1)

	go func() {
		result, err := app.TriggerNow()
		first <- outcome{result: result, err: err}
	}()

	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		require.Fail(t, "the first run never reached the bot endpoint")
	}

	// the guard answers the second caller at once instead of queueing it up.
	_, err := app.TriggerNow()
	require.ErrorIs(t, err, ErrInformRunning)

	close(release)

	done := <-first
	require.NoError(t, done.err)
	require.NotNil(t, done.result)
	assert.True(t, done.result.Success)
	assert.True(t, done.result.Notified)
}

func TestInformRunningReportsARunInFlight(t *testing.T) {
	t.Parallel()

	reached := make(chan struct{}, 1)
	release := make(chan struct{})

	// the bot endpoint blocks until released, so the run provably sits inside
	// the guard while the state is read.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != blockedBotPath {
			http.NotFound(w, r)

			return
		}

		select {
		case reached <- struct{}{}:
		default:
		}

		<-release

		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	app := newTestApp(t)
	require.NoError(t, app.SaveConfig(validFeedDTO()))
	require.NoError(t, app.SaveWebhook(server.URL+blockedBotPath))

	assert.False(t, app.InformRunning(), "an idle app reports no run")

	done := make(chan struct{})

	go func() {
		defer close(done)

		_, _ = app.TriggerNow()
	}()

	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		require.Fail(t, "the run never reached the bot endpoint")
	}

	assert.True(t, app.InformRunning(), "the state holds while the run is in flight")

	close(release)
	<-done

	assert.False(t, app.InformRunning(), "the state clears once the run finished")
}
