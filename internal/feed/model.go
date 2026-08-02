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

const (
	StatusNormal = 1
	StatusError  = 2
)

// Parse types of a Source. An explicit legal value decides the parser;
// an empty or unknown value keeps falling back to the historical derivation.
const (
	ParseTypeFeed  = "feed"
	ParseTypeRegex = "regex"
	ParseTypeJSON  = "json"
)

const (
	// DefaultCategoryID is the id of the seeded "未分类" category.
	DefaultCategoryID = 1

	// DefaultCategoryName is the name of the seeded default category.
	DefaultCategoryName = "未分类"
)

// Category groups sources. Listing order is Sort ascending, then ID.
type Category struct {
	ID   int64  `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	Name string `json:"name" gorm:"uniqueIndex"`
	Sort int    `json:"sort" gorm:"index"`
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
	Status        int    `json:"status"`
	ErrorInfo     string `json:"error_info"`
	ParseType     string `json:"parse_type"`
	CategoryID    int64  `json:"category_id" gorm:"index"`
	Enabled       bool   `json:"enabled" gorm:"index"`
}

// IsLegalParseType reports whether the value is one of the supported parse types.
func IsLegalParseType(parseType string) bool {
	switch parseType {
	case ParseTypeFeed, ParseTypeRegex, ParseTypeJSON:
		return true
	default:
		return false
	}
}

// ResolveParseType returns the parser to use for the source.
// The explicit ParseType wins when it is legal; otherwise the historical
// derivation from IsJSON/Regex is kept as a long lived fallback.
func (s *Source) ResolveParseType() string {
	if IsLegalParseType(s.ParseType) {
		return s.ParseType
	}

	return DeriveParseType(s)
}

// DeriveParseType derives the parse type from the legacy fields only.
func DeriveParseType(s *Source) string {
	switch {
	case s.IsJSON:
		return ParseTypeJSON
	case s.Regex != "":
		return ParseTypeRegex
	default:
		return ParseTypeFeed
	}
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
	SourceID  int64  `json:"source_id" gorm:"index"`

	// FetchedAt is the unix second the article was first persisted by a fetch.
	// It stays NULL for articles stored before this field existed.
	FetchedAt *int64 `json:"fetched_at" gorm:"index"`

	// InformedAt is the unix second a notification actually delivered the article.
	// It stays NULL for historical articles, even when Informed is already true.
	InformedAt *int64 `json:"informed_at" gorm:"index"`
}
