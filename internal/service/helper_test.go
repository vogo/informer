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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/service"
)

// windowsGOOS is the platform that does not model unix permission bits.
const windowsGOOS = "windows"

// atomFeed is a minimal but valid atom document served by the test feed endpoint.
const atomFeed = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>test feed</title>
  <updated>2026-08-01T10:00:00Z</updated>
  <entry>
    <title>first article</title>
    <link href="https://example.com/first"/>
    <updated>2026-08-01T10:00:00Z</updated>
  </entry>
  <entry>
    <title>second article</title>
    <link href="https://example.com/second"/>
    <updated>2026-08-01T09:00:00Z</updated>
  </entry>
</feed>`

// regexPage is the html document the regex source is parsed from.
const regexPage = `<html><body>
<a href="/posts/one" class="post">regex one</a>
<a href="/posts/two" class="post">regex two</a>
</body></html>`

// jsonPage is the document the json source is parsed from.
const jsonPage = `{"data":{"items":[
{"title":"json one","url":"https://example.com/json-one"},
{"title":"json two","url":"https://example.com/json-two"}]}}`

// newContentServer serves the three parseable documents plus a broken endpoint.
func newContentServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/atom.xml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(atomFeed))
	})
	mux.HandleFunc("/page.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(regexPage))
	})
	mux.HandleFunc("/api.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonPage))
	})
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	// the bot endpoints carry the lark host in their path so that the notification
	// step routes to the real client code while staying inside the test server.
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

// newService opens a service on a fresh data directory holding an informer.json.
func newService(t *testing.T) *service.Service {
	t.Helper()

	dir := t.TempDir()
	writeConfigFile(t, dir, &inform.Config{Feed: &feed.Config{
		MaxInformFeedSize: 10,
		FeedExpireDays:    3650,
		SameSiteMaxCount:  5,
		MaxFetchNum:       10,
	}})

	svc, err := service.New(dir)
	require.NoError(t, err)

	return svc
}

func writeConfigFile(t *testing.T, dir string, config *inform.Config) {
	t.Helper()

	raw, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, writeRaw(dir, string(raw)))
}

// writeRaw stores arbitrary informer.json content, including invalid documents.
func writeRaw(dir, content string) error {
	return os.WriteFile(filepath.Join(dir, inform.ConfigFileName), []byte(content), 0o600)
}

// feedSource builds a source pointing at the atom endpoint of the test server.
func feedSource(server *httptest.Server) *feed.Source {
	return &feed.Source{
		Title:     "atom source",
		URL:       server.URL + "/atom.xml",
		Weight:    50,
		ParseType: feed.ParseTypeFeed,
	}
}

// regexSource builds a source parsed with a regex.
func regexSource(server *httptest.Server) *feed.Source {
	return &feed.Source{
		Title:     "regex source",
		URL:       server.URL + "/page.html",
		Weight:    40,
		Regex:     `<a href="([^"]+)" class="post">([^<]+)</a>`,
		TitleExp:  "$2",
		URLExp:    "$1",
		ParseType: feed.ParseTypeRegex,
	}
}

// jsonSource builds a source parsed from a json document.
func jsonSource(server *httptest.Server) *feed.Source {
	return &feed.Source{
		Title:         "json source",
		URL:           server.URL + "/api.json",
		Weight:        30,
		IsJSON:        true,
		JsonTitlePath: "data/items[]/title",
		JsonURLPath:   "data/items[]/url",
		ParseType:     feed.ParseTypeJSON,
	}
}

// dbSnapshot is the full content of every table the service touches.
type dbSnapshot struct {
	Sources    []feed.Source
	Articles   []feed.Article
	Categories []feed.Category
	Configs    []feed.Config
}

// snapshotDB reads every relevant table so a caller can prove nothing changed.
func snapshotDB(t *testing.T, svc *service.Service) dbSnapshot {
	t.Helper()

	var snapshot dbSnapshot

	require.NoError(t, svc.DB().Order("id").Find(&snapshot.Sources).Error)
	require.NoError(t, svc.DB().Order("id").Find(&snapshot.Articles).Error)
	require.NoError(t, svc.DB().Order("id").Find(&snapshot.Categories).Error)
	require.NoError(t, svc.DB().Order("id").Find(&snapshot.Configs).Error)

	return snapshot
}
