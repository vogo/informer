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

package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/cli"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// regexPage is the document the "feed parse" regression test parses.
const regexPage = `<html><body>
<a href="/posts/one" class="post">regex one</a>
<a href="/posts/two" class="post">regex two</a>
</body></html>`

func newTestService(t *testing.T) *service.Service {
	t.Helper()

	svc, err := service.New(t.TempDir())
	require.NoError(t, err)

	return svc
}

// runFeed executes a feed command and captures what it printed.
func runFeed(t *testing.T, svc *service.Service, ops ...string) (string, error) {
	t.Helper()

	original := os.Stdout

	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = writer

	runErr := cli.Feed(svc, ops)

	require.NoError(t, writer.Close())

	os.Stdout = original

	out, err := io.ReadAll(reader)
	require.NoError(t, err)

	return string(out), runErr
}

// TestFeedCommandsGoThroughService is the CLI regression: every command keeps its
// historical output while the work now happens in the service layer.
func TestFeedCommandsGoThroughService(t *testing.T) {
	svc := newTestService(t)

	out, err := runFeed(t, svc, "add", "阮一峰blog", "http://www.ruanyifeng.com/blog/atom.xml")
	require.NoError(t, err)
	assert.Equal(t, "1,\t阮一峰blog,\thttp://www.ruanyifeng.com/blog/atom.xml\n", out)

	out, err = runFeed(t, svc, "list")
	require.NoError(t, err)
	assert.Equal(t, "1,\t阮一峰blog,\thttp://www.ruanyifeng.com/blog/atom.xml\n", out)

	_, err = runFeed(t, svc, "update", "1", "weight", "80")
	require.NoError(t, err)

	_, err = runFeed(t, svc, "update", "1", "max_fetch_num", "1")
	require.NoError(t, err)

	out, err = runFeed(t, svc, "view", "1")
	require.NoError(t, err)
	assert.Contains(t, out, "id:\t1\n")
	assert.Contains(t, out, "title:\t阮一峰blog\n")
	assert.Contains(t, out, "weight:\t80\n")
	assert.Contains(t, out, "max_fetch_num:\t1\n")
	// the new columns show up in the same view, without changing the old lines.
	assert.Contains(t, out, "parse_type:\tfeed\n")
	assert.Contains(t, out, "category_id:\t1\n")
	assert.Contains(t, out, "enabled:\ttrue\n")

	out, err = runFeed(t, svc, "copy", "1")
	require.NoError(t, err)
	assert.Contains(t, out, "2,\t阮一峰blog,")

	stored, err := svc.GetSource(2)
	require.NoError(t, err)
	assert.Equal(t, int64(80), stored.Weight, "a copy keeps the settings of its origin")
	assert.True(t, stored.Enabled)

	_, err = runFeed(t, svc, "remove", "2")
	require.NoError(t, err)

	out, err = runFeed(t, svc, "list")
	require.NoError(t, err)
	assert.NotContains(t, out, "2,\t")
}

func TestFeedParseUsesPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(regexPage))
	}))
	defer server.Close()

	svc := newTestService(t)

	source := &feed.Source{
		Title:    "regex source",
		URL:      server.URL,
		Regex:    `<a href="([^"]+)" class="post">([^<]+)</a>`,
		TitleExp: "$2",
		URLExp:   "$1",
	}
	require.NoError(t, svc.CreateSource(source))

	before, err := svc.GetSource(source.ID)
	require.NoError(t, err)

	out, err := runFeed(t, svc, "parse", "1")
	require.NoError(t, err)
	assert.Contains(t, out, "regex one : "+server.URL+"/posts/one\n")
	assert.Contains(t, out, "regex two : "+server.URL+"/posts/two\n")

	// parsing from the CLI is a preview: it stores nothing and changes no status.
	after, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	page, err := svc.ListArticles(service.ArticleQuery{}, service.PageRequest{})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestFeedCategoryCommand(t *testing.T) {
	svc := newTestService(t)

	out, err := runFeed(t, svc, "category")
	require.NoError(t, err)
	assert.Equal(t, "1,\t未分类,\t0\n", out)
}

func TestFeedUsageErrors(t *testing.T) {
	svc := newTestService(t)

	require.NoError(t, cli.Feed(svc, nil), "an empty command list is a no-op, as before")

	for _, ops := range [][]string{
		{"add", "only-title"},
		{"update", "1", "weight"},
		{"view"},
		{"remove"},
		{"parse"},
		{"copy"},
		{"unknown"},
		{"view", "not-a-number"},
	} {
		require.ErrorIs(t, cli.Feed(svc, ops), cli.ErrUsage, "ops: %v", ops)
	}
}
