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

package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

func TestRebuildHistoryIndexOnEmptyHomeDoesNothing(t *testing.T) {
	svc := newService(t)

	result, err := svc.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Zero(t, result.Days)
	assert.Zero(t, result.Links)
	assert.Zero(t, result.Filled)
	assert.Zero(t, result.Failed)
	assert.Empty(t, result.Errors)
}

func TestRebuildHistoryIndexFillsOnlyProvableMatches(t *testing.T) {
	svc := newService(t)

	matched := storeArticle(t, svc, "https://example.com/matched", nil)
	alreadyIndexed := storeArticle(t, svc, "https://example.com/indexed", ptrInt64(1234))
	duplicateA := storeArticle(t, svc, "https://example.com/duplicate", nil)
	duplicateB := storeArticle(t, svc, "https://example.com/duplicate", nil)
	untouched := storeArticle(t, svc, "https://example.com/never-reported", nil)

	writeDaily(t, svc, "2026-01-10", ""+
		"recommended:\n"+
		"- one match, https://example.com/matched\n"+
		"- already stamped, https://example.com/indexed\n"+
		"- two rows share it, https://example.com/duplicate\n"+
		"- not in the database, https://example.com/unknown\n")

	result, err := svc.RebuildHistoryIndex()
	require.NoError(t, err)

	assert.Equal(t, 1, result.Days)
	assert.Equal(t, 4, result.Links)
	assert.Equal(t, 1, result.Filled)
	assert.Equal(t, 1, result.SkippedAlreadyIndexed)
	assert.Equal(t, 1, result.SkippedAmbiguous)
	assert.Equal(t, 1, result.SkippedUnmatched)
	assert.Equal(t, 3, result.Skipped())
	assert.Zero(t, result.Failed)

	// the single unambiguous hit got the start of the report day.
	//nolint:gosmopolitan //a daily file is named with the local date, so it is read back in it.
	expected := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.Local).Unix()
	assert.Equal(t, &expected, reloadArticle(t, svc, matched).InformedAt)

	// nothing else was touched: no overwrite, no invented history.
	assert.Equal(t, ptrInt64(1234), reloadArticle(t, svc, alreadyIndexed).InformedAt)
	assert.Nil(t, reloadArticle(t, svc, duplicateA).InformedAt)
	assert.Nil(t, reloadArticle(t, svc, duplicateB).InformedAt)
	assert.Nil(t, reloadArticle(t, svc, untouched).InformedAt)
}

func TestRebuildHistoryIndexIsIdempotent(t *testing.T) {
	svc := newService(t)

	id := storeArticle(t, svc, "https://example.com/first", nil)
	writeDaily(t, svc, "2026-01-10", "- first report, https://example.com/first\n")

	first, err := svc.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Equal(t, 1, first.Filled)

	stamped := reloadArticle(t, svc, id).InformedAt
	require.NotNil(t, stamped)

	second, err := svc.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Zero(t, second.Filled)
	assert.Equal(t, 1, second.SkippedAlreadyIndexed)

	// a repeated run must leave the value it wrote the first time alone.
	assert.Equal(t, stamped, reloadArticle(t, svc, id).InformedAt)
}

func TestRebuildHistoryIndexCreditsTheEarliestReport(t *testing.T) {
	svc := newService(t)

	id := storeArticle(t, svc, "https://example.com/repeated", nil)
	writeDaily(t, svc, "2026-02-20", "- seen again, https://example.com/repeated\n")
	writeDaily(t, svc, "2026-01-05", "- seen first, https://example.com/repeated\n")

	result, err := svc.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Equal(t, 2, result.Days)
	assert.Equal(t, 1, result.Filled)

	//nolint:gosmopolitan //a daily file is named with the local date, so it is read back in it.
	expected := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.Local).Unix()
	assert.Equal(t, &expected, reloadArticle(t, svc, id).InformedAt)
}

func TestRebuildHistoryIndexParsesMarkdownLinkForms(t *testing.T) {
	svc := newService(t)

	plain := storeArticle(t, svc, "https://example.com/plain", nil)
	linked := storeArticle(t, svc, "https://example.com/linked", nil)
	sentence := storeArticle(t, svc, "https://example.com/sentence", nil)

	// the last line ends the url with a chinese full stop, the punctuation a hand
	// written note leaves behind and that the extractor has to trim.
	writeDaily(t, svc, "2026-04-01", ""+
		"- plain, https://example.com/plain\n"+
		"- [markdown link](https://example.com/linked)\n"+
		"see https://example.com/sentence\u3002\n")

	result, err := svc.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Equal(t, 3, result.Filled)

	for _, id := range []int64{plain, linked, sentence} {
		assert.NotNil(t, reloadArticle(t, svc, id).InformedAt)
	}
}

// storeArticle inserts one article and returns its id.
func storeArticle(t *testing.T, svc *service.Service, url string, informedAt *int64) int64 {
	t.Helper()

	article := &feed.Article{URL: url, Title: url, InformedAt: informedAt}
	require.NoError(t, svc.CreateArticle(article))

	return article.ID
}

func reloadArticle(t *testing.T, svc *service.Service, id int64) *feed.Article {
	t.Helper()

	article, err := svc.GetArticle(id)
	require.NoError(t, err)

	return article
}

func ptrInt64(value int64) *int64 {
	return &value
}
