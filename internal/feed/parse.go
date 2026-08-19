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
	"time"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/runlog"
)

// ParseArticles parses a source with the parser its ParseType selects, falling back
// to the historical derivation when no legal parse type is set.
//
// agentConfig backs a ParseTypeAgent source and is ignored by every other parser;
// a nil value runs the agent with its defaults.
//
// It performs network reads and agent runs only: no source, article, status,
// timestamp or config record is created or modified, so it can be called repeatedly
// without changing any persisted state.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func ParseArticles(source *Source, agentConfig *agent.Config, sink runlog.Sink) ([]*Article, error) {
	parseType := source.ResolveParseType()

	runlog.Infof(sink, "开始解析「%s」，类型：%s", source.Title, parseType)

	started := time.Now()

	articles, err := parseArticlesBy(parseType, source, agentConfig, sink)
	elapsed := time.Since(started).Round(time.Millisecond)

	if err != nil {
		runlog.Errorf(sink, "解析失败，耗时 %s：%v", elapsed, err)

		return nil, err
	}

	runlog.Infof(sink, "解析完成，%d 条，耗时 %s", len(articles), elapsed)

	return articles, nil
}

// parseArticlesBy dispatches to the parser of one parse type.
func parseArticlesBy(parseType string, source *Source, agentConfig *agent.Config,
	sink runlog.Sink,
) ([]*Article, error) {
	switch parseType {
	case ParseTypeJSON:
		return JsonParse(source, sink)
	case ParseTypeRegex:
		return RegexParse(source, sink)
	case ParseTypeAgent:
		return AgentParse(source, agentConfig, sink)
	default:
		return GoFeedArticles(source, sink)
	}
}
