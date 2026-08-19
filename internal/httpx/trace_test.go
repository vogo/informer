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

package httpx_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/httpx"
)

func TestGetLinkDataStatsReportsTheExchange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, "<html>hello</html>")
	}))
	defer server.Close()

	data, stats, err := httpx.GetLinkDataStats(server.URL)

	require.NoError(t, err)
	require.Equal(t, "<html>hello</html>", string(data))
	require.Equal(t, http.StatusOK, stats.StatusCode)
	require.Equal(t, len(data), stats.Bytes)
	require.Contains(t, stats.ContentType, "text/html")
	require.Equal(t, server.URL, stats.URL)
	require.Positive(t, stats.Duration)
}

func TestGetLinkDataStatsReportsARefusedPage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "gone", http.StatusNotFound)
	}))
	defer server.Close()

	// a 404 body still comes back: the point of the stats is that the caller can
	// now tell that apart from a page that really had no articles.
	data, stats, err := httpx.GetLinkDataStats(server.URL)

	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, stats.StatusCode)
	require.Contains(t, string(data), "gone")
}

func TestGetLinkDataKeepsItsSignature(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "body")
	}))
	defer server.Close()

	data, err := httpx.GetLinkData(server.URL)

	require.NoError(t, err)
	require.Equal(t, "body", string(data))
}

func TestNewTracingClientReportsEveryExchange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/atom+xml")
		_, _ = io.WriteString(writer, "<feed/>")
	}))
	defer server.Close()

	var seen []*httpx.FetchStats

	client := httpx.NewTracingClient(func(stats *httpx.FetchStats) {
		seen = append(seen, stats)
	})

	response, err := client.Get(server.URL)
	require.NoError(t, err)

	_, _ = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())

	require.Len(t, seen, 1)
	require.Equal(t, http.StatusOK, seen[0].StatusCode)
	require.Equal(t, "application/atom+xml", seen[0].ContentType)
	require.Equal(t, server.URL, seen[0].URL)
}

func TestNewTracingClientWithoutATraceIsTheSharedClient(t *testing.T) {
	t.Parallel()

	require.Same(t, httpx.HTTPClient, httpx.NewTracingClient(nil))
}
