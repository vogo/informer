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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/runlog"
)

// recorder keeps what one traced parse reported.
type recorder struct {
	mu      sync.Mutex
	entries []runlog.Entry
}

func (r *recorder) Write(entry runlog.Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, entry)
}

func (r *recorder) document() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	texts := make([]string, 0, len(r.entries))
	for _, entry := range r.entries {
		texts = append(texts, entry.Level+" "+entry.Text)
	}

	return strings.Join(texts, "\n")
}

// The two article page every regex case here parses, and the expressions that
// pull it apart. They are constants because the same literals appear in more
// than one case.
const (
	linkFixture = `<a href="/a">First</a><a href="/b">Second</a>`
	linkRegex   = `<a href="([^"]+)">([^<]+)</a>`
	firstGroup  = "$1"
	secondGroup = "$2"
)

// regexSource is a source that reads the given address with the fixture regex.
func regexSource(url string) *feed.Source {
	return &feed.Source{
		Title:     "fixture",
		URL:       url,
		ParseType: feed.ParseTypeRegex,
		Regex:     linkRegex,
		URLExp:    firstGroup,
		TitleExp:  secondGroup,
	}
}

//nolint:gosmopolitan //the recorded lines this asserts on are chinese by design.
func TestParseArticlesRecordsAWholeRegexRun(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(writer, linkFixture)
	}))
	defer server.Close()

	sink := &recorder{}

	articles, err := feed.ParseArticles(regexSource(server.URL), nil, sink)
	require.NoError(t, err)
	require.Len(t, articles, 2)

	document := sink.document()
	require.Contains(t, document, "开始解析「fixture」，类型：regex")
	require.Contains(t, document, "GET "+server.URL+" → 200")
	require.Contains(t, document, "正则匹配到 2 组")
	require.Contains(t, document, "第 1 条：First | "+server.URL+"/a")
	require.Contains(t, document, "解析完成，2 条")
}

//nolint:gosmopolitan //the recorded lines this asserts on are chinese by design.
func TestParseArticlesReportsARefusedPage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "not here", http.StatusNotFound)
	}))
	defer server.Close()

	sink := &recorder{}

	_, err := feed.ParseArticles(regexSource(server.URL), nil, sink)
	require.ErrorIs(t, err, feed.ErrNoRegexMatch)

	document := sink.document()
	// the status code is what says "the regex is fine, the page is a 404".
	require.Contains(t, document, runlog.LevelWarn+" GET "+server.URL+" → 404")
	require.Contains(t, document, "正则没有匹配到内容")
	require.Contains(t, document, "解析失败")
}

func TestRegexParseRefusesAHalfConfiguredSource(t *testing.T) {
	t.Parallel()

	sink := &recorder{}

	// this used to return "no articles and no error", which reads in the window
	// as a source that simply had nothing to offer.
	_, err := feed.RegexParse(&feed.Source{URL: "https://a.com", Regex: "x", TitleExp: firstGroup}, sink)

	require.ErrorIs(t, err, feed.ErrNoURLExp)
	require.Contains(t, sink.document(), runlog.LevelError)
}

//nolint:gosmopolitan //asserting on the shipped chinese narration.
func TestJsonParseReportsAPathThatFindsNothing(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":{"items":[{"t":"a","u":"https://a.com/1"}]}}`)
	}))
	defer server.Close()

	sink := &recorder{}

	_, err := feed.JsonParse(&feed.Source{
		URL:           server.URL,
		JsonTitlePath: "data/items[]/t",
		JsonURLPath:   "data/nope[]/u",
	}, sink)

	require.ErrorIs(t, err, feed.ErrJSONPathMismatch)
	require.Contains(t, sink.document(), "标题路径 data/items[]/t 取到 1 个")
}

func TestJsonParseSkipsAValueThatIsNotAString(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"items":[{"t":1,"u":"https://a.com/1"},{"t":"b","u":"https://a.com/2"}]}`)
	}))
	defer server.Close()

	sink := &recorder{}

	// a numeric title used to be a naked type assertion, and a panic here takes
	// the whole desktop window down with it.
	articles, err := feed.JsonParse(&feed.Source{
		URL:           server.URL,
		JsonTitlePath: "items[]/t",
		JsonURLPath:   "items[]/u",
	}, sink)

	require.NoError(t, err)
	require.Len(t, articles, 1)
	require.Equal(t, "b", articles[0].Title)
	require.Contains(t, sink.document(), runlog.LevelWarn)
}

func TestParseArticlesWithoutASinkStillParses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, linkFixture)
	}))
	defer server.Close()

	articles, err := feed.ParseArticles(regexSource(server.URL), nil, nil)

	require.NoError(t, err)
	require.Len(t, articles, 2)
}
