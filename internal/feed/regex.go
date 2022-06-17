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

	"github.com/vogo/logger"
	"github.com/wongoo/informer/internal/httpx"
)

func RegexParse(source *Source) ([]*Article, error) {
	re, err := regexp.Compile(source.Regex)
	if err != nil {
		return nil, err
	}

	data, err := httpx.GetLinkData(source.URL)
	if err != nil {
		return nil, err
	}

	hostPrefix := GetHostPrefix(source.URL)

	var articles []*Article

	match := re.FindAllSubmatch(data, -1)

	for _, groups := range match {
		article := matchArticle(source, hostPrefix, groups)
		if article != nil {
			articles = append(articles, article)
		}
	}

	return articles, nil
}

func matchArticle(source *Source, hostPrefix string, groups [][]byte) *Article {
	defer func() {
		if err := recover(); err != nil {
			logger.Errorf("match article error: %v", err)
		}
	}()

	link := string(groups[source.URLGroup])
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		if link[0] != '/' {
			link = hostPrefix + "/" + link
		} else {
			link = hostPrefix + link
		}
	}

	return &Article{
		URL:       link,
		Title:     string(groups[source.TitleGroup]),
		Timestamp: time.Now().Unix(),
		Weight:    source.Weight,
	}
}
