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

package service

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/vogo/informer/internal/feed"
)

// urlPattern extracts the article links out of a daily markdown file. The stored
// format is "- <title>, <url>", but a hand edited file may also carry markdown
// links, so the closing characters of those forms end the match as well.
var urlPattern = regexp.MustCompile(`https?://[^\s<>"'()\[\]]+`) //nolint:gochecknoglobals //compiled once on purpose.

const (
	// urlLookupChunk bounds the size of one "url IN (...)" lookup.
	urlLookupChunk = 400

	// maxReportedErrors bounds the error list carried back to the caller, so a
	// broken data directory reports a readable summary instead of thousands of lines.
	maxReportedErrors = 20
)

// HistoryIndexResult reports what one history index rebuild did.
//
// Filled plus every Skipped* counter plus Failed equals Links: each extracted link
// ends up in exactly one bucket, so the numbers add up in the UI.
type HistoryIndexResult struct {
	// Days is the number of daily markdown files read.
	Days int `json:"days"`

	// Links is the number of distinct article links found across those files.
	Links int `json:"links"`

	// Filled is the number of articles that got their missing InformedAt.
	Filled int `json:"filled"`

	// SkippedAlreadyIndexed counts links whose article already carried an InformedAt.
	// Those values are never overwritten, which is what makes a rerun a no-op.
	SkippedAlreadyIndexed int `json:"skipped_already_indexed"`

	// SkippedUnmatched counts links with no article of that exact url in the database.
	SkippedUnmatched int `json:"skipped_unmatched"`

	// SkippedAmbiguous counts links matching more than one article, where no single
	// record can be filled without guessing.
	SkippedAmbiguous int `json:"skipped_ambiguous"`

	// Failed counts links whose update hit an error, plus the files that could not be read.
	Failed int `json:"failed"`

	// Errors carries at most maxReportedErrors messages describing the failures.
	Errors []string `json:"errors"`
}

// Skipped is the total number of links deliberately left untouched.
func (r *HistoryIndexResult) Skipped() int {
	return r.SkippedAlreadyIndexed + r.SkippedUnmatched + r.SkippedAmbiguous
}

// RebuildHistoryIndex fills the missing InformedAt of articles that provably appeared
// in a stored daily report.
//
// The only evidence used is the article url printed in the daily markdown: a link is
// matched against the exact url of a stored article, and only a single unambiguous hit
// is filled. Links with no match, links matching several articles, and articles that
// already carry an InformedAt are all left exactly as they are - the operation never
// invents a history it cannot prove, and it never overwrites a recorded one.
//
// The timestamp written is the local start of the report day, so the value is derived
// from the file name alone. Repeating the rebuild therefore changes nothing: everything
// filled by the first run is reported as already indexed by the next.
func (s *Service) RebuildHistoryIndex() (*HistoryIndexResult, error) {
	result := &HistoryIndexResult{Errors: []string{}}

	years, err := s.DailyIndex()
	if err != nil {
		return nil, err
	}

	// oldest day first, so a link printed on several days is credited to the first
	// report that carried it, whatever order the directory listing came back in.
	firstSeen := make(map[string]int64)

	for _, day := range flattenDailyDays(years) {
		content, contentErr := s.DailyContent(day)
		if contentErr != nil {
			result.Failed++
			result.appendError(fmt.Sprintf("read daily report %s: %v", day, contentErr))

			continue
		}

		result.Days++

		parsed, parseErr := parseDay(day)
		if parseErr != nil {
			// DailyIndex only ever yields canonical dates; keep the guard anyway.
			result.Failed++
			result.appendError(parseErr.Error())

			continue
		}

		informedAt := parsed.Unix()

		for _, link := range extractLinks(content) {
			if _, ok := firstSeen[link]; !ok {
				firstSeen[link] = informedAt
			}
		}
	}

	result.Links = len(firstSeen)

	s.applyHistoryLinks(firstSeen, result)

	return result, nil
}

// applyHistoryLinks resolves every extracted link to an article and fills the ones
// that can be matched without guessing.
func (s *Service) applyHistoryLinks(firstSeen map[string]int64, result *HistoryIndexResult) {
	links := make([]string, 0, len(firstSeen))
	for link := range firstSeen {
		links = append(links, link)
	}

	// a stable order keeps the reported errors reproducible between runs.
	sort.Strings(links)

	for start := 0; start < len(links); start += urlLookupChunk {
		end := min(start+urlLookupChunk, len(links))
		chunk := links[start:end]

		byURL, err := s.articlesByURL(chunk)
		if err != nil {
			result.Failed += len(chunk)
			result.appendError(err.Error())

			continue
		}

		for _, link := range chunk {
			s.applyHistoryLink(link, firstSeen[link], byURL[link], result)
		}
	}
}

// applyHistoryLink classifies one link and fills the single article it can prove.
func (s *Service) applyHistoryLink(link string, informedAt int64, matches []*feed.Article, result *HistoryIndexResult) {
	switch {
	case len(matches) == 0:
		result.SkippedUnmatched++

		return
	case len(matches) > 1:
		result.SkippedAmbiguous++

		return
	}

	article := matches[0]
	if article.InformedAt != nil {
		result.SkippedAlreadyIndexed++

		return
	}

	// the "informed_at IS NULL" guard repeats the check inside the statement, so a
	// concurrent inform run that just stamped the article wins and is reported as
	// already indexed instead of being overwritten.
	update := s.db.Model(&feed.Article{}).
		Where("id = ? AND informed_at IS NULL", article.ID).
		Update("informed_at", informedAt)
	if update.Error != nil {
		result.Failed++
		result.appendError(fmt.Sprintf("fill inform time of article %d (%s): %v", article.ID, link, update.Error))

		return
	}

	if update.RowsAffected == 0 {
		result.SkippedAlreadyIndexed++

		return
	}

	result.Filled++
}

// articlesByURL groups the stored articles of the given urls by url.
func (s *Service) articlesByURL(urls []string) (map[string][]*feed.Article, error) {
	var articles []*feed.Article

	err := s.db.Model(&feed.Article{}).Where("url IN ?", urls).Order("id asc").Find(&articles).Error
	if err != nil {
		return nil, fmt.Errorf("look up %d article urls: %w", len(urls), err)
	}

	byURL := make(map[string][]*feed.Article, len(articles))
	for _, article := range articles {
		byURL[article.URL] = append(byURL[article.URL], article)
	}

	return byURL, nil
}

// appendError records a failure message up to the reporting bound.
func (r *HistoryIndexResult) appendError(message string) {
	if len(r.Errors) >= maxReportedErrors {
		return
	}

	r.Errors = append(r.Errors, message)
}

// flattenDailyDays returns every indexed day, oldest first.
func flattenDailyDays(years []*DailyYear) []string {
	var days []string

	for _, year := range years {
		for _, month := range year.Months {
			for _, day := range month.Days {
				days = append(days, day.Date)
			}
		}
	}

	sort.Strings(days)

	return days
}

// extractLinks returns the distinct article links of one daily markdown file,
// with the punctuation a sentence or a markdown link leaves on the tail removed.
func extractLinks(content string) []string {
	matches := urlPattern.FindAllString(content, -1)

	seen := make(map[string]struct{}, len(matches))
	links := make([]string, 0, len(matches))

	for _, match := range matches {
		link := strings.TrimRight(match, ".,;:!?、，。；：！？")
		if link == "" {
			continue
		}

		if _, ok := seen[link]; ok {
			continue
		}

		seen[link] = struct{}{}

		links = append(links, link)
	}

	return links
}
