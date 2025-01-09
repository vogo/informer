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

package feed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJsonParseFeed(t *testing.T) {
	src := &Source{
		ID:            0,
		Title:         "test",
		URL:           "https://newsapi-hbr.caijingmobile.com/topic/detail?topic_id=3101&type=0&last_id=",
		CURL:          "",
		Weight:        50,
		MaxFetchNum:   100,
		Regex:         "",
		TitleExp:      "",
		URLExp:        "",
		Redirect:      false,
		Sort:          false,
		IsJSON:        true,
		JsonTitlePath: "data/article_list[]/article/title",
		JsonURLPath:   "data/article_list[]/article/share_url",
	}
	articles, err := JsonParseFeed(nil, src, time.Now().Unix())
	assert.Nil(t, err)
	assert.NotNil(t, articles)
	for _, a := range articles {
		t.Log(a)
	}
}
