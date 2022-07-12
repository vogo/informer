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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wongoo/informer/internal/feed"
)

func TestRegexParse(t *testing.T) {
	t.Parallel()

	articles, err := feed.RegexParse(&feed.Source{
		URL:         "https://kaifa.baidu.com/rest/v1/home/github?optionLanguage=go&optionSince=DAILY",
		Weight:      50,
		MaxFetchNum: 5,
		Regex:       `,"url":"([^"]+)","title":"([^"]+)",`,
		TitleExp:    "$2",
		URLExp:      "$1",
	})

	assert.Nil(t, err)

	for _, a := range articles {
		t.Log(a.Title, a.URL)
	}
}

func TestRegexParse2(t *testing.T) {
	t.Parallel()

	articles, err := feed.RegexParse(&feed.Source{
		URL:         "https://www.infoq.cn/profile/7A6A18227E53FA/publish/article",
		Weight:      50,
		MaxFetchNum: 5,
		Regex:       `<h6[^>]+class="favorite"><a[^>]+ href="([^"]+)" target="_blank" rel="" class="com-article-title"><!----> ([^<>]+) </a></h6>`,
		TitleExp:    "$2",
		URLExp:      "$1",
	})

	assert.Nil(t, err)

	for _, a := range articles {
		t.Log(a.Title, a.URL)
	}
}

func TestRegexParse3(t *testing.T) {
	t.Parallel()

	articles, err := feed.RegexParse(&feed.Source{
		URL:         "https://www.infoq.cn/topic/architecture",
		CURL:        `curl 'https://www.infoq.cn/public/v1/article/getList' -H 'Origin: https://www.infoq.cn' -H 'Referer: https://www.infoq.cn/topic/architecture' --data-raw '{"type":1,"size":30,"id":8}' --compressed`,
		Weight:      50,
		MaxFetchNum: 5,
		Regex:       `"article_title":"([^"]+)".*?"uuid":"([^"]+)"`,
		TitleExp:    "$1",
		URLExp:      "https://www.infoq.cn/article/$2",
	})

	assert.Nil(t, err)

	for _, a := range articles {
		t.Log(a.Title, a.URL)
	}
}
