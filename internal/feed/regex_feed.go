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

import "github.com/vogo/logger"

func regexParseFeed(config *Config, source *Source, _ int64) {
	logger.Info("regex parse feed: ", source.URL)

	articles, err := RegexParse(source)
	if err != nil {
		logger.Infof("regex parse feed url error! url: %s, error: %v", source.URL, err)

		return
	}

	count := 0

	for _, a := range articles {
		if isFeedURLExists(a.URL) {
			continue
		}

		feedDataDB.Save(a)

		count++

		if source.MaxFetchNum > 0 {
			if count >= source.MaxFetchNum {
				break
			}
		} else if config.MaxFetchNum > 0 && count >= config.MaxFetchNum {
			break
		}
	}
}
