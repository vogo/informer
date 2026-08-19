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
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/runlog"
	"github.com/vogo/informer/internal/service"
)

func TestPreviewParsers(t *testing.T) {
	server := newContentServer(t)

	cases := []struct {
		name   string
		source func(*httptest.Server) *feed.Source
		titles []string
		urls   []string
	}{
		{
			name:   "feed",
			source: feedSource,
			titles: []string{"first article", "second article"},
			urls:   []string{"https://example.com/first", "https://example.com/second"},
		},
		{
			name:   "regex",
			source: regexSource,
			titles: []string{"regex one", "regex two"},
			urls:   []string{server.URL + "/posts/one", server.URL + "/posts/two"},
		},
		{
			name:   "json",
			source: jsonSource,
			titles: []string{"json one", "json two"},
			urls:   []string{"https://example.com/json-one", "https://example.com/json-two"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			svc := newService(t)

			source := testCase.source(server)
			require.NoError(t, svc.CreateSource(source))

			articles, err := svc.Preview(source.ID)
			require.NoError(t, err)
			require.Len(t, articles, len(testCase.titles))

			for i, article := range articles {
				assert.Equal(t, testCase.titles[i], article.Title)
				assert.Equal(t, testCase.urls[i], article.URL)
			}
		})
	}
}

// TestPreviewUsesImplicitFallback proves the historical derivation still decides
// when a source carries no explicit parse type.
func TestPreviewUsesImplicitFallback(t *testing.T) {
	server := newContentServer(t)

	cases := []struct {
		name      string
		source    func(*httptest.Server) *feed.Source
		wantTitle string
	}{
		{name: "regex from Regex field", source: regexSource, wantTitle: "regex one"},
		{name: "json from IsJSON field", source: jsonSource, wantTitle: "json one"},
		{name: "feed as the last resort", source: feedSource, wantTitle: "first article"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			svc := newService(t)

			source := testCase.source(server)
			source.ParseType = ""
			require.NoError(t, svc.CreateSource(source))

			articles, err := svc.Preview(source.ID)
			require.NoError(t, err)
			require.NotEmpty(t, articles)
			assert.Equal(t, testCase.wantTitle, articles[0].Title)
		})
	}
}

// TestPreviewUnknownParseTypeFallsBack covers a historical value the enum does not know.
func TestPreviewUnknownParseTypeFallsBack(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	source := regexSource(server)
	source.ParseType = ""
	require.NoError(t, svc.CreateSource(source))

	// an unknown value can only reach the database directly, never through Create.
	require.NoError(t, svc.DB().Model(&feed.Source{}).Where("id = ?", source.ID).
		Update("parse_type", "rss-v9").Error)

	articles, err := svc.Preview(source.ID)
	require.NoError(t, err)
	require.NotEmpty(t, articles)
	assert.Equal(t, "regex one", articles[0].Title)
}

func TestPreviewReportsParseFailure(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	source := regexSource(server)
	source.URL = server.URL + "/atom.xml" // the regex matches nothing in an atom document.
	require.NoError(t, svc.CreateSource(source))

	_, err := svc.Preview(source.ID)
	require.ErrorIs(t, err, feed.ErrNoRegexMatch)
}

func TestPreviewErrorPropagation(t *testing.T) {
	svc := newService(t)

	_, err := svc.Preview(404)
	require.ErrorIs(t, err, service.ErrNotFound)

	_, err = svc.PreviewSource(nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = svc.PreviewSource(&feed.Source{Title: "no address"})
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

// TestPreviewChangesNoPersistedState is the core purity guarantee: a preview may be
// repeated any number of times without altering a single stored row, including the
// health status of a source whose parsing fails.
func TestPreviewChangesNoPersistedState(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateCategory(&feed.Category{Name: "技术", Sort: 3}))
	require.NoError(t, svc.CreateConfig(&feed.Config{MaxInformFeedSize: 7}))

	ok := feedSource(server)
	require.NoError(t, svc.CreateSource(ok))

	broken := regexSource(server)
	broken.URL = server.URL + "/atom.xml"
	require.NoError(t, svc.CreateSource(broken))

	missing := feedSource(server)
	missing.URL = server.URL + "/broken"
	require.NoError(t, svc.CreateSource(missing))

	require.NoError(t, svc.CreateArticle(&feed.Article{URL: "https://example.com/first", Title: "already stored"}))

	before := snapshotDB(t, svc)

	for range 3 {
		articles, err := svc.Preview(ok.ID)
		require.NoError(t, err)
		require.Len(t, articles, 2)

		_, err = svc.Preview(broken.ID)
		require.Error(t, err)

		_, err = svc.Preview(missing.ID)
		require.Error(t, err)
	}

	assert.Equal(t, before, snapshotDB(t, svc),
		"preview must leave every source, article, category and config untouched")
}

// TestPreviewIgnoresEnabledFlag: a disabled source is excluded from real fetches
// but must still be previewable.
func TestPreviewIgnoresEnabledFlag(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	source := feedSource(server)
	require.NoError(t, svc.CreateSource(source))
	require.NoError(t, svc.SetSourceEnabled(source.ID, false))

	articles, err := svc.Preview(source.ID)
	require.NoError(t, err)
	assert.Len(t, articles, 2)
}

// TestPreviewSourceWorksWithoutStoredRecord lets a caller try a source out before saving it.
func TestPreviewSourceWorksWithoutStoredRecord(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	before := snapshotDB(t, svc)

	articles, err := svc.PreviewSource(feedSource(server))
	require.NoError(t, err)
	assert.Len(t, articles, 2)

	assert.Equal(t, before, snapshotDB(t, svc))
}

// previewRecorder keeps what one traced preview reported.
type previewRecorder struct {
	entries []runlog.Entry
}

func (r *previewRecorder) Write(entry runlog.Entry) {
	r.entries = append(r.entries, entry)
}

func (r *previewRecorder) document() string {
	texts := make([]string, 0, len(r.entries))
	for _, entry := range r.entries {
		texts = append(texts, entry.Text)
	}

	return strings.Join(texts, "\n")
}

//nolint:gosmopolitan //the recorded lines this asserts on are chinese by design.
func TestPreviewTracedRecordsTheRun(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	source := regexSource(server)
	require.NoError(t, svc.CreateSource(source))

	sink := &previewRecorder{}

	articles, err := svc.PreviewTraced(source.ID, sink)
	require.NoError(t, err)
	assert.Len(t, articles, 2)

	document := sink.document()
	assert.Contains(t, document, "开始解析")
	assert.Contains(t, document, "GET "+server.URL)
	assert.Contains(t, document, "解析完成，2 条")
}

// TestPreviewTracedWithoutASinkMatchesPreview keeps the untraced path honest: a
// caller with nothing to show must get exactly what it got before.
func TestPreviewTracedWithoutASinkMatchesPreview(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	source := feedSource(server)
	require.NoError(t, svc.CreateSource(source))

	traced, err := svc.PreviewTraced(source.ID, nil)
	require.NoError(t, err)

	plain, err := svc.Preview(source.ID)
	require.NoError(t, err)

	require.Len(t, traced, len(plain))

	for i := range plain {
		assert.Equal(t, plain[i].Title, traced[i].Title)
		assert.Equal(t, plain[i].URL, traced[i].URL)
	}
}

//nolint:gosmopolitan //the recorded lines this asserts on are chinese by design.
func TestPreviewSourceTracedRecordsAFailure(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	broken := regexSource(server)
	broken.Regex = `<never-matches ([^"]+)>`

	sink := &previewRecorder{}

	_, err := svc.PreviewSourceTraced(broken, sink)
	require.Error(t, err)

	assert.Contains(t, sink.document(), "正则没有匹配到内容")
}
