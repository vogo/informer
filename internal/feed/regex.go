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
	"bytes"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/vogo/vogo/vnet/vurl"
	"github.com/vogo/vogo/vregexp"

	"github.com/vogo/informer/internal/runlog"
)

// Errors of a regex source.
var (
	// ErrNoRegexMatch is returned when the source regex matches nothing.
	// The caller decides whether that marks the source unhealthy; parsing itself
	// never writes to the database so that Preview stays side effect free.
	ErrNoRegexMatch = errors.New("no match")

	// ErrNoURLExp and ErrNoTitleExp mark a half configured regex source. They are
	// errors rather than an empty result because "no articles" and "this source
	// was never finished" are different answers, and only one of them is worth
	// showing the user as a failure.
	ErrNoURLExp   = errors.New("url exp is empty")
	ErrNoTitleExp = errors.New("title exp is empty")
)

// RegexParse parses the source with its regex. It performs network reads only
// and leaves every persisted record untouched.
//
// sink, when not nil, receives the run's progress as it happens; nil parses
// exactly as before and reports nothing.
//
//nolint:gosmopolitan //the recorded lines are read by a chinese user.
func RegexParse(source *Source, sink runlog.Sink) ([]*Article, error) {
	re, err := regexp.Compile(source.Regex)
	if err != nil {
		runlog.Errorf(sink, "正则编译失败：%v", err)

		return nil, err
	}

	if source.URLExp == "" {
		runlog.Errorf(sink, "%v, url: %s", ErrNoURLExp, source.URL)

		return nil, ErrNoURLExp
	}

	if source.TitleExp == "" {
		runlog.Errorf(sink, "%v, url: %s", ErrNoTitleExp, source.URL)

		return nil, ErrNoTitleExp
	}

	data, err := readURLData(source, sink)
	if err != nil {
		return nil, err
	}

	match := re.FindAllSubmatch(data, -1)
	if len(match) == 0 {
		runlog.Warnf(sink, "正则没有匹配到内容，url: %s，正则: %s，响应开头：%s",
			source.URL, source.Regex, runlog.Truncate(string(data), maxLoggedBodyRunes))

		return nil, ErrNoRegexMatch
	}

	runlog.Infof(sink, "正则匹配到 %d 组", len(match))

	return regexArticles(source, match, sink), nil
}

// regexArticles turns the matched groups into articles, up to the source's fetch
// limit.
//
//nolint:gosmopolitan //the recorded lines are read by a chinese user.
func regexArticles(source *Source, match [][][]byte, sink runlog.Sink) []*Article {
	urlRegexRender := vregexp.RegexGroupRender(source.URLExp)
	linkParser := func(groups [][]byte) string {
		return string(urlRegexRender(groups))
	}

	titleRegexRender := vregexp.RegexGroupRender(source.TitleExp)
	titleParser := func(groups [][]byte) string {
		t := bytes.TrimSpace(titleRegexRender(groups))
		s := string(t)

		return strings.Join(strings.Fields(s), " ")
	}

	hostPrefix := GetHostPrefix(source.URL)

	//nolint:prealloc //ignore this.
	var articles []*Article

	for i, groups := range match {
		if source.MaxFetchNum > 0 && i >= source.MaxFetchNum {
			break
		}

		link := adjustLink(hostPrefix, linkParser(groups))
		title := titleParser(groups)

		runlog.Infof(sink, "第 %d 条：%s | %s", i+1, title, link)

		if source.Redirect {
			link = vurl.RedirectURL(link)
		}

		articles = append(articles, &Article{
			URL:       link,
			Title:     title,
			Timestamp: time.Now().Unix(),
			Weight:    source.Weight,
			SourceID:  source.ID,
		})
	}

	return articles
}
