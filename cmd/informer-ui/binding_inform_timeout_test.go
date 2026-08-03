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

// The tests in this file deliberately stay sequential: they swap the outbound
// clients of ding, lark and soup to shorten the production 60s deadline, and
// an overlapping parallel trigger test that reads those clients would race
// with the swap.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/ding"
	"github.com/vogo/informer/internal/lark"
	"github.com/vogo/informer/internal/soup"
)

// frozenBotServer serves accepting bots and freezes every other path until the
// test ends, so one trigger can hang on a delivery while the next one proves
// the guard is free again.
func frozenBotServer(t *testing.T) *httptest.Server {
	t.Helper()

	release := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/feishu.cn/ok", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0}`))
	})
	mux.HandleFunc("/dingtalk.com/ok", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":0}`))
	})
	mux.HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		<-release
	})

	server := httptest.NewServer(mux)
	// cleanups run LIFO: release the frozen handlers first, so server.Close
	// does not wait on requests that never answer.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	return server
}

// shortenDeliveryDeadlines swaps the outbound clients of the inform run for a
// short deadline - including the daily sentence behind a frozen endpoint - and
// returns one restore covering all of them, so a frozen request fails within
// the test instead of parking it for the full production minute.
func shortenDeliveryDeadlines(frozenSoupURL string) func() {
	short := &http.Client{Timeout: 100 * time.Millisecond}

	restoreLark := lark.SetClientForTest(short)
	restoreDing := ding.SetClientForTest(short)
	restoreSoupClient := soup.SetClientForTest(short)
	restoreSoupURL := soup.SetURLForTest(frozenSoupURL)

	return func() {
		restoreLark()
		restoreDing()
		restoreSoupClient()
		restoreSoupURL()
	}
}

func TestTriggerNowReleasesTheGuardAfterAFrozenLarkWebhook(t *testing.T) {
	assertFrozenTriggerReleasesTheGuard(t, "/feishu.cn/hang", "/feishu.cn/ok")
}

func TestTriggerNowReleasesTheGuardAfterAFrozenDingWebhook(t *testing.T) {
	assertFrozenTriggerReleasesTheGuard(t, "/dingtalk.com/hang", "/dingtalk.com/ok")
}

// assertFrozenTriggerReleasesTheGuard proves the failure mode behind the
// permanent "already in progress" fault: a webhook that never answers must
// fail the run within the client deadline, release the process guard on the
// way out, and leave the next manual push free to run at once.
func assertFrozenTriggerReleasesTheGuard(t *testing.T, frozenPath, healthyPath string) {
	t.Helper()

	server := frozenBotServer(t)
	app := newTestApp(t)

	require.NoError(t, app.SaveConfig(validFeedDTO()))
	require.NoError(t, app.SaveWebhook(server.URL+frozenPath))

	restore := shortenDeliveryDeadlines(server.URL)
	defer restore()

	start := time.Now()
	result, err := app.TriggerNow()
	elapsed := time.Since(start)

	require.NoError(t, err, "a frozen webhook is a run failure, not a binding failure")
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.False(t, result.Notified)
	assert.NotEmpty(t, result.ErrorInfo)
	assert.Less(t, elapsed, 30*time.Second, "the client deadline must bound a frozen webhook")
	assert.FileExists(t, result.ContentFilePath, "the daily file survives a failed delivery")

	// the frozen run must have released the guard on its way out: with a
	// healthy webhook the very next push runs at once instead of failing with
	// ErrInformRunning until a restart.
	require.NoError(t, app.SaveWebhook(server.URL+healthyPath))

	next, err := app.TriggerNow()
	require.NotErrorIs(t, err, ErrInformRunning, "a failed run must free the guard")
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.True(t, next.Success)
	assert.True(t, next.Notified)
}
