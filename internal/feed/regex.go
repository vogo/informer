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
	"regexp"
	"strings"
	"time"

	"github.com/wongoo/informer/internal/httpx"
	"github.com/wongoo/informer/internal/util"
)

func RegexParse(source *Source) ([]*Article, error) {
	re, err := regexp.Compile(source.Regex)
	if err != nil {
		return nil, err
	}

	linkParser := func(groups [][]byte) string {
		return string(groups[source.URLGroup])
	}

	titleParser := func(groups [][]byte) string {
		return string(groups[source.TitleGroup])
	}

	if source.URLExp != "" {
		urlRegexRender := util.RegexMatchRender(source.URLExp)
		linkParser = func(groups [][]byte) string {
			return string(urlRegexRender(groups))
		}
	}

	if source.TitleExp != "" {
		titleRegexRender := util.RegexMatchRender(source.TitleExp)
		titleParser = func(groups [][]byte) string {
			return string(titleRegexRender(groups))
		}
	}

	data, err := httpx.GetLinkData(source.URL)
	if err != nil {
		return nil, err
	}

	hostPrefix := GetHostPrefix(source.URL)

	// nolint:prealloc //ignore this.
	var articles []*Article

	match := re.FindAllSubmatch(data, -1)

	for i, groups := range match {
		if source.MaxFetchNum > 0 && i >= source.MaxFetchNum {
			break
		}

		link := linkParser(groups)
		link = adjustLink(hostPrefix, link)
		title := titleParser(groups)

		article := &Article{
			URL:       link,
			Title:     title,
			Timestamp: time.Now().Unix(),
			Weight:    source.Weight,
		}

		articles = append(articles, article)
	}

	return articles, nil
}

func adjustLink(hostPrefix, link string) string {
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		if link[0] != '/' {
			link = hostPrefix + "/" + link
		} else {
			link = hostPrefix + link
		}
	}

	return link
}
