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

type Config struct {
	ID                int64 `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	MaxInformFeedSize int   `json:"max_inform_feed_size"`
	FeedExpireDays    int   `json:"feed_expire_days"`
	SameSiteMaxCount  int   `json:"same_site_max_count"`
	MaxFetchNum       int   `json:"max_fetch_num"`
}

type Source struct {
	ID            int64  `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	CURL          string `json:"curl"`
	Weight        int64  `json:"weight"`
	MaxFetchNum   int    `json:"max_fetch_num"`
	Regex         string `json:"regex"`
	TitleExp      string `json:"title_exp"`
	URLExp        string `json:"url_exp"`
	Redirect      bool   `json:"redirect"` // if redirect the parsed url
	Sort          bool   `json:"sort"`     // whether sort the result
	IsJSON        bool   `json:"is_json"`
	JsonTitlePath string `json:"json_title_path"`
	JsonURLPath   string `json:"json_url_path"`
}

type Detail struct{}

type Article struct {
	ID        int64  `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	URL       string `json:"url" gorm:"index"`
	Title     string `json:"title"`
	Timestamp int64  `json:"timestamp"`
	Weight    int64  `json:"weight"`
	Informed  bool   `json:"informed" gorm:"index"`
	Score     int64  `json:"score" gorm:"index"`
}
