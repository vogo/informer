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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vogo/informer/internal/feed"
)

// regexFixtureHTML keeps the markup shape the historical live target had, so
// the regex parse stays verified without depending on a third-party site that
// can change or block CI at any time.
const regexFixtureHTML = `<html><body>
<h6 class="favorite"><a data-v="7e6a1822" href="/article/armageddon" target="_blank" rel="" class="com-article-title"><!----> Armageddon </a></h6>
<h6 class="favorite"><a data-v="7e6a1822" href="/article/life-planning" target="_blank" rel="" class="com-article-title"><!----> What to do when you have no idea </a></h6>
</body></html>`

func TestRegexParse(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("network test")
	}

	articles, err := feed.RegexParse(&feed.Source{
		URL:         "https://kaifa.baidu.com/rest/v1/home/github?optionLanguage=go&optionSince=DAILY",
		Weight:      50,
		MaxFetchNum: 5,
		Regex:       `,"url":"([^"]+)","title":"([^"]+)",`,
		TitleExp:    "$2",
		URLExp:      "$1",
	}, nil)

	assert.Nil(t, err)

	for _, a := range articles {
		t.Log(a.Title, a.URL)
	}
}

func TestRegexParse2(t *testing.T) {
	t.Parallel()

	// the historical live target became a client rendered page, so the same
	// parse now runs against a local fixture with the identical markup shape.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(regexFixtureHTML))
	}))
	defer server.Close()

	articles, err := feed.RegexParse(&feed.Source{
		URL:         server.URL,
		Weight:      50,
		MaxFetchNum: 5,
		Regex:       `<h6[^>]+class="favorite"><a[^>]+ href="([^"]+)" target="_blank" rel="" class="com-article-title"><!----> ([^<>]+) </a></h6>`,
		TitleExp:    "$2",
		URLExp:      "$1",
	}, nil)

	assert.Nil(t, err)

	if assert.Len(t, articles, 2) {
		assert.Equal(t, "Armageddon", articles[0].Title)
		assert.Equal(t, server.URL+"/article/armageddon", articles[0].URL)
		assert.Equal(t, "What to do when you have no idea", articles[1].Title)
		assert.Equal(t, server.URL+"/article/life-planning", articles[1].URL)
	}
}

func TestRegexParse3(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("network test")
	}

	articles, err := feed.RegexParse(&feed.Source{
		URL:         "https://www.infoq.cn/topic/architecture",
		CURL:        `curl 'https://www.infoq.cn/public/v1/article/getList' -H 'Origin: https://www.infoq.cn' -H 'Referer: https://www.infoq.cn/topic/architecture' --data-raw '{"type":1,"size":30,"id":8}' --compressed`,
		Weight:      50,
		MaxFetchNum: 5,
		Regex:       `"article_title":"([^"]+)".*?"uuid":"([^"]+)"`,
		TitleExp:    "$1",
		URLExp:      "https://www.infoq.cn/article/$2",
	}, nil)

	assert.Nil(t, err)

	for _, a := range articles {
		t.Log(a.Title, a.URL)
	}
}
