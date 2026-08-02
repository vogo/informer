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

// ParseArticles parses a source with the parser its ParseType selects, falling back
// to the historical derivation when no legal parse type is set.
//
// It performs network reads only: no source, article, status, timestamp or config
// record is created or modified, so it can be called repeatedly without changing
// any persisted state.
func ParseArticles(source *Source) ([]*Article, error) {
	switch source.ResolveParseType() {
	case ParseTypeJSON:
		return JsonParse(source)
	case ParseTypeRegex:
		return RegexParse(source)
	default:
		return GoFeedArticles(source)
	}
}
