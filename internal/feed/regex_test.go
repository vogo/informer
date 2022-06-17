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
		TitleGroup:  2,
		URLGroup:    1,
	})

	assert.Nil(t, err)

	for _, a := range articles {
		t.Log(a.Title, a.URL)
	}
}

func TestRegexParse2(t *testing.T) {
	t.Parallel()

	articles, err := feed.RegexParse(&feed.Source{
		URL:         "https://www.yinwang.org",
		Weight:      50,
		MaxFetchNum: 5,
		Regex:       `<li class="list-group-item title">[\W]*<div class="date">[^<]+</div><br>[\W]*<a href="([^"]+)">([^<>]+)</a>[\W]*</li>`,
		TitleGroup:  2,
		URLGroup:    1,
	})

	assert.Nil(t, err)

	for _, a := range articles {
		t.Log(a.Title, a.URL)
	}
}
