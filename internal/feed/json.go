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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vogo/logger"
	"github.com/vogo/vogo/vnet/vurl"

	"github.com/vogo/informer/internal/runlog"
)

// Errors of a json source. They replace what used to be a silent empty result:
// a path that points nowhere and a page that really has no articles look the
// same in an empty list, and only one of them is the user's mistake.
var (
	ErrJSONUnmarshal    = errors.New("response is not valid json")
	ErrJSONPathMismatch = errors.New("title path and url path yield different counts")
	ErrJSONPathEmpty    = errors.New("title path and url path yield nothing")
)

// jsonItem is one title and link pair read out of a json document.
type jsonItem struct {
	title string
	link  string
}

// jsonEntry reads one title and link pair, refusing a value that is not a
// string. A path aimed at a number used to panic here, which takes the whole
// desktop window with it.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func jsonEntry(titleValue, urlValue any, sink runlog.Sink) (jsonItem, bool) {
	link, ok := urlValue.(string)
	if !ok {
		runlog.Warnf(sink, "链接不是字符串，已跳过：%v", urlValue)

		return jsonItem{}, false
	}

	title, ok := titleValue.(string)
	if !ok {
		runlog.Warnf(sink, "标题不是字符串，已跳过：%v", titleValue)

		return jsonItem{}, false
	}

	return jsonItem{title: title, link: link}, true
}

func jsonParseFeed(config *Config, source *Source, _ int64) {
	articles, err := JsonParse(source, nil)
	if err != nil {
		logger.Infof("regex parse feed url error! url: %s, error: %v", source.URL, err)

		updateSourceError(source, err)

		return
	}

	updateSourceNormal(source)

	saveParsedArticles(config, source, articles)
}

//nolint:revive,gosmopolitan //historical exported name; the recorded lines speak the user's language.
func JsonParse(source *Source, sink runlog.Sink) ([]*Article, error) {
	data, err := readURLData(source, sink)
	if err != nil {
		return nil, err
	}

	var jsonData map[string]interface{}
	if jsonErr := json.Unmarshal(data, &jsonData); jsonErr != nil {
		runlog.Errorf(sink, "响应不是合法 JSON：%v，响应开头：%s",
			jsonErr, runlog.Truncate(string(data), maxLoggedBodyRunes))

		return nil, fmt.Errorf("%w: %w", ErrJSONUnmarshal, jsonErr)
	}

	titles := getJSONNestedValue(jsonData, source.JsonTitlePath, sink)
	urls := getJSONNestedValue(jsonData, source.JsonURLPath, sink)

	runlog.Infof(sink, "标题路径 %s 取到 %d 个，链接路径 %s 取到 %d 个",
		source.JsonTitlePath, len(titles), source.JsonURLPath, len(urls))

	if len(titles) != len(urls) {
		runlog.Errorf(sink, "%v: titles %d, urls %d", ErrJSONPathMismatch, len(titles), len(urls))

		return nil, ErrJSONPathMismatch
	}

	if len(titles) == 0 {
		runlog.Warnf(sink, "%v，url: %s", ErrJSONPathEmpty, source.URL)

		return nil, ErrJSONPathEmpty
	}

	return jsonArticles(source, titles, urls, sink), nil
}

// jsonArticles turns the extracted title and url values into articles, up to the
// source's fetch limit.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func jsonArticles(source *Source, titles, urls []any, sink runlog.Sink) []*Article {
	hostPrefix := GetHostPrefix(source.URL)

	//nolint:prealloc //ignore this.
	var articles []*Article

	for i, titleValue := range titles {
		if source.MaxFetchNum > 0 && i >= source.MaxFetchNum {
			break
		}

		item, ok := jsonEntry(titleValue, urls[i], sink)
		if !ok {
			continue
		}

		link := adjustLink(hostPrefix, item.link)

		runlog.Infof(sink, "第 %d 条：%s | %s", i+1, item.title, link)

		if source.Redirect {
			link = vurl.RedirectURL(link)
		}

		articles = append(articles, &Article{
			URL:       link,
			Title:     item.title,
			Timestamp: time.Now().Unix(),
			Weight:    source.Weight,
			SourceID:  source.ID,
		})
	}

	return articles
}

func getJSONNestedValue(data map[string]any, keyPath string, sink runlog.Sink) []any {
	keys := strings.Split(keyPath, "/")

	return parseJSONNestedValue(data, keys, sink)
}

//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func parseJSONNestedValue(data map[string]any, keys []string, sink runlog.Sink) []any {
	var values []interface{}

	keyName := keys[0]
	key := keyName
	isArr := strings.HasSuffix(keyName, "[]")
	if isArr {
		key = keyName[:len(keyName)-2]
	}

	switch v := data[key].(type) {
	case map[string]interface{}:
		if len(keys) == 1 {
			runlog.Warnf(sink, "路径 %s 指向的是对象而不是取值：%v", keys, v)
		} else {
			values = appendNoneNil(values, parseJSONNestedValue(v, keys[1:], sink))
		}
	case []interface{}:
		if isArr {
			for _, item := range v {
				if len(keys) == 1 {
					values = append(values, item)
				} else {
					values = appendNoneNil(values, parseJSONNestedValue(item.(map[string]any), keys[1:], sink))
				}
			}
		} else {
			if len(keys) == 1 {
				values = append(values, v)
			} else {
				values = appendNoneNil(values, parseJSONNestedValue(v[0].(map[string]any), keys[1:], sink))
			}
		}
	default:
		if len(keys) == 1 {
			values = append(values, v)
		} else {
			runlog.Warnf(sink, "路径 %s 没有取到值", keys)
		}
	}

	return values
}

func appendNoneNil(values []interface{}, value []interface{}) []interface{} {
	if len(value) > 0 {
		values = append(values, value...)
	}
	return values
}
