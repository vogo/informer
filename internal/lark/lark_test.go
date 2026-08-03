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

package lark_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/lark"
)

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

func TestLarkDeliversToAnAcceptingBot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()

	require.NoError(t, lark.Lark(server.URL, "content"))
}

func TestLarkReportsARejectedMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bot rejected the message", http.StatusInternalServerError)
	}))
	defer server.Close()

	err := lark.Lark(server.URL, "content")
	require.ErrorIs(t, err, lark.ErrLarkResponse, "a non successful status stays a bot rejection")
}

// TestLarkFailsAFrozenWebhookWithinTheTimeout covers the stuck lock root cause:
// a webhook that hangs must fail the delivery within the client deadline
// instead of blocking the inform run forever.
func TestLarkFailsAFrozenWebhookWithinTheTimeout(t *testing.T) {
	server := frozenServer(t)

	restore := lark.SetClientForTest(&http.Client{Timeout: 100 * time.Millisecond})
	defer restore()

	start := time.Now()
	err := lark.Lark(server.URL, "content")
	elapsed := time.Since(start)

	require.Error(t, err, "a frozen webhook must surface as a delivery failure")
	require.ErrorContains(t, err, "post lark message")
	require.NotErrorIs(t, err, lark.ErrLarkResponse, "a timeout is not a bot rejection")
	assert.Less(t, elapsed, 30*time.Second, "the client deadline must bound a frozen webhook")
}
