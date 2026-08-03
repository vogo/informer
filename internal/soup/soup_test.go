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

package soup_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/soup"
)

func TestGetDailySoupReturnsTheContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":"hello soup"}`))
	}))
	defer server.Close()

	restore := soup.SetURLForTest(server.URL)
	defer restore()

	require.Equal(t, "hello soup", soup.GetDailySoup())
}

func TestGetDailySoupDegradesOnARejectedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusInternalServerError)
	}))
	defer server.Close()

	restore := soup.SetURLForTest(server.URL)
	defer restore()

	require.Empty(t, soup.GetDailySoup(), "a non successful status degrades to empty")
}

func TestGetDailySoupDegradesOnInvalidContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not a json document"))
	}))
	defer server.Close()

	restore := soup.SetURLForTest(server.URL)
	defer restore()

	require.Empty(t, soup.GetDailySoup(), "an unparsable body degrades to empty")
}

// TestGetDailySoupDegradesWhenTheEndpointFreezes covers the pre-delivery hang:
// a frozen endpoint must degrade to empty within the client deadline instead
// of occupying the inform run forever.
func TestGetDailySoupDegradesWhenTheEndpointFreezes(t *testing.T) {
	release := make(chan struct{})

	frozen := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	// cleanups run LIFO: release the frozen handler first, so frozen.Close
	// does not wait on a request that never answers.
	t.Cleanup(frozen.Close)
	t.Cleanup(func() { close(release) })

	restoreURL := soup.SetURLForTest(frozen.URL)
	defer restoreURL()

	restoreClient := soup.SetClientForTest(&http.Client{Timeout: 100 * time.Millisecond})
	defer restoreClient()

	start := time.Now()
	content := soup.GetDailySoup()
	elapsed := time.Since(start)

	assert.Empty(t, content, "a frozen endpoint degrades to empty")
	assert.Less(t, elapsed, 30*time.Second, "the client deadline must bound a frozen endpoint")
}
