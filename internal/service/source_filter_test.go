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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// The titles below name the shape of each seeded record, so an expectation
// reads as the set of cards a sidebar filter should leave visible.
const (
	titleExplicitFeed  = "explicit-feed"
	titleExplicitJSON  = "explicit-json"
	titleExplicitRegex = "explicit-regex"
	titleLegacyFeed    = "legacy-feed"
	titleLegacyJSON    = "legacy-json"
	titleLegacyRegex   = "legacy-regex"
	titleUnknownRegex  = "unknown-type-regex-fallback"
	titleUnknownFeed   = "unknown-type-feed-fallback"

	titleNormal       = "fetched-normally"
	titleFailed       = "fetch-failed"
	titleNeverFetched = "never-fetched"
	titleOddStatus    = "unknown-status-value"

	titleWanted   = "matches-every-dimension"
	titleDisabled = "same-but-disabled"
)

// Columns the create API neither accepts nor validates, written directly below.
const (
	columnParseType = "parse_type"
	columnStatus    = "status"

	// unknownParseType is a value a build supporting another parser could have
	// stored; this build has to fall back to the legacy fields for it.
	unknownParseType = "rss-v9"
)

// titlesOf flattens a listing to the titles, in the order the page renders them.
func titlesOf(sources []*feed.Source) []string {
	titles := make([]string, 0, len(sources))
	for _, source := range sources {
		titles = append(titles, source.Title)
	}

	return titles
}

// storeSource creates a subscription and then writes the columns the create API
// refuses or ignores - an unknown parse type, a fetch health state - directly,
// which is the shape older builds left in the database. The url is derived from
// the title because only its uniqueness matters here.
func storeSource(t *testing.T, svc *service.Service, source *feed.Source, columns map[string]any) *feed.Source {
	t.Helper()

	if source.URL == "" {
		source.URL = "https://example.com/" + source.Title
	}

	require.NoError(t, svc.CreateSource(source))

	for column, value := range columns {
		require.NoError(t, svc.UpdateSourceColumn(source.ID, column, value))
	}

	stored, err := svc.GetSource(source.ID)
	require.NoError(t, err)

	return stored
}

// seedParseTypeSources covers every route through ResolveParseType: an explicit
// type, an empty type falling back to each legacy field, and an unknown type
// that still has to fall back rather than vanish from the filters.
func seedParseTypeSources(t *testing.T, svc *service.Service) {
	t.Helper()

	storeSource(t, svc, &feed.Source{Title: titleExplicitFeed, ParseType: feed.ParseTypeFeed}, nil)

	// an explicit type wins over both legacy fields.
	storeSource(t, svc, &feed.Source{Title: titleExplicitJSON, ParseType: feed.ParseTypeJSON, Regex: "x"}, nil)
	storeSource(t, svc, &feed.Source{Title: titleExplicitRegex, ParseType: feed.ParseTypeRegex, IsJSON: true}, nil)

	// the historical records: no parse type at all.
	storeSource(t, svc, &feed.Source{Title: titleLegacyJSON, IsJSON: true}, nil)
	storeSource(t, svc, &feed.Source{Title: titleLegacyRegex, Regex: "x"}, nil)
	storeSource(t, svc, &feed.Source{Title: titleLegacyFeed}, nil)

	// a parse type written by a build that supported a parser this one does not.
	storeSource(t, svc, &feed.Source{Title: titleUnknownRegex, Regex: "x"},
		map[string]any{columnParseType: unknownParseType})
	storeSource(t, svc, &feed.Source{Title: titleUnknownFeed},
		map[string]any{columnParseType: unknownParseType})
}

func TestAllSourcesFiltersByResolvedParseType(t *testing.T) {
	svc := newService(t)

	seedParseTypeSources(t, svc)

	// the filter has to agree with the value the list renders per record, or the
	// cards would show a type the sidebar did not select.
	all, err := svc.AllSources(service.SourceQuery{})
	require.NoError(t, err)
	require.Len(t, all, 8)

	for _, parseType := range feed.LegalParseTypes() {
		expected := make([]string, 0, len(all))

		for _, source := range all {
			if source.ResolveParseType() == parseType {
				expected = append(expected, source.Title)
			}
		}

		filtered, err := svc.AllSources(service.SourceQuery{ParseType: parseType})
		require.NoError(t, err)
		assert.Equal(t, expected, titlesOf(filtered), "parse type %q", parseType)
	}

	// spelled out, so a regression in ResolveParseType cannot quietly move the
	// expectation above along with the implementation.
	jsonSources, err := svc.AllSources(service.SourceQuery{ParseType: feed.ParseTypeJSON})
	require.NoError(t, err)
	assert.Equal(t, []string{titleExplicitJSON, titleLegacyJSON}, titlesOf(jsonSources))

	regexSources, err := svc.AllSources(service.SourceQuery{ParseType: feed.ParseTypeRegex})
	require.NoError(t, err)
	assert.Equal(t, []string{titleExplicitRegex, titleLegacyRegex, titleUnknownRegex}, titlesOf(regexSources))

	feedSources, err := svc.AllSources(service.SourceQuery{ParseType: feed.ParseTypeFeed})
	require.NoError(t, err)
	assert.Equal(t, []string{titleExplicitFeed, titleLegacyFeed, titleUnknownFeed}, titlesOf(feedSources))
}

func TestListSourcesParseTypeFilterKeepsPagingSemantics(t *testing.T) {
	svc := newService(t)

	seedParseTypeSources(t, svc)

	page, err := svc.ListSources(
		service.SourceQuery{ParseType: feed.ParseTypeRegex},
		service.PageRequest{Offset: 0, Limit: 2},
	)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)

	// the total counts the filtered set, not the table.
	assert.Equal(t, int64(3), page.Total)
	assert.True(t, page.HasMore)
	assert.Equal(t, []string{titleExplicitRegex, titleLegacyRegex}, titlesOf(page.Items))
	assert.Less(t, page.Items[0].ID, page.Items[1].ID, "a filtered page keeps the id ascending order")
}

func TestAllSourcesFiltersByFetchStatus(t *testing.T) {
	svc := newService(t)

	storeSource(t, svc, &feed.Source{Title: titleNormal}, map[string]any{columnStatus: feed.StatusNormal})
	storeSource(t, svc, &feed.Source{Title: titleFailed}, map[string]any{columnStatus: feed.StatusError})
	storeSource(t, svc, &feed.Source{Title: titleNeverFetched}, nil)

	// a value no current build writes still has to land in a bucket the sidebar
	// offers; the cards label it "未抓取" too.
	storeSource(t, svc, &feed.Source{Title: titleOddStatus}, map[string]any{columnStatus: 7})

	normal, err := svc.AllSources(service.SourceQuery{FetchStatus: service.FetchStatusNormal})
	require.NoError(t, err)
	assert.Equal(t, []string{titleNormal}, titlesOf(normal))

	failed, err := svc.AllSources(service.SourceQuery{FetchStatus: service.FetchStatusError})
	require.NoError(t, err)
	assert.Equal(t, []string{titleFailed}, titlesOf(failed))

	unfetched, err := svc.AllSources(service.SourceQuery{FetchStatus: service.FetchStatusUnfetched})
	require.NoError(t, err)
	assert.Equal(t, []string{titleNeverFetched, titleOddStatus}, titlesOf(unfetched))
}

func TestSourceQueryRejectsUnknownFilterValues(t *testing.T) {
	svc := newService(t)

	// an unknown enum is a mistake worth reporting, never an alias of "all".
	const (
		absentParseType   = "agent"
		absentFetchStatus = "half-broken"
	)

	_, err := svc.AllSources(service.SourceQuery{ParseType: absentParseType})
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = svc.AllSources(service.SourceQuery{FetchStatus: absentFetchStatus})
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = svc.ListSources(service.SourceQuery{ParseType: absentParseType}, service.PageRequest{})
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = svc.ListSources(service.SourceQuery{FetchStatus: absentFetchStatus}, service.PageRequest{})
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func TestAllSourcesCombinesEveryFilterWithAnd(t *testing.T) {
	svc := newService(t)

	category := &feed.Category{Name: "filter-combination", Sort: 1}
	require.NoError(t, svc.CreateCategory(category))

	// the one record that satisfies all four dimensions: in the category, json
	// through the legacy flag only, failed fetch, still enabled.
	wanted := storeSource(t, svc,
		&feed.Source{Title: titleWanted, CategoryID: category.ID, IsJSON: true},
		map[string]any{columnStatus: feed.StatusError})
	assert.Equal(t, feed.ParseTypeJSON, wanted.ResolveParseType())
	assert.Empty(t, wanted.ParseType, "the record keeps deriving its type from the legacy flag")

	// each of the following differs from it in exactly one dimension.
	storeSource(t, svc, &feed.Source{Title: "other-category", IsJSON: true},
		map[string]any{columnStatus: feed.StatusError})
	storeSource(t, svc, &feed.Source{Title: "other-type", CategoryID: category.ID, Regex: "x"},
		map[string]any{columnStatus: feed.StatusError})
	storeSource(t, svc, &feed.Source{Title: "other-status", CategoryID: category.ID, IsJSON: true},
		map[string]any{columnStatus: feed.StatusNormal})

	disabled := storeSource(t, svc,
		&feed.Source{Title: titleDisabled, CategoryID: category.ID, IsJSON: true},
		map[string]any{columnStatus: feed.StatusError})
	require.NoError(t, svc.SetSourceEnabled(disabled.ID, false))

	enabled := true
	combined := service.SourceQuery{
		CategoryID:  category.ID,
		ParseType:   feed.ParseTypeJSON,
		FetchStatus: service.FetchStatusError,
		Enabled:     &enabled,
	}

	matched, err := svc.AllSources(combined)
	require.NoError(t, err)
	assert.Equal(t, []string{titleWanted}, titlesOf(matched))

	page, err := svc.ListSources(combined, service.PageRequest{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	assert.Equal(t, []string{titleWanted}, titlesOf(page.Items))

	// dropping the enabled dimension widens the result by exactly the disabled
	// record, which proves the dimensions are combined rather than merged.
	combined.Enabled = nil

	widened, err := svc.AllSources(combined)
	require.NoError(t, err)
	assert.Equal(t, []string{titleWanted, titleDisabled}, titlesOf(widened))

	// a keyword still narrows the same combination.
	combined.Keyword = titleWanted

	narrowed, err := svc.AllSources(combined)
	require.NoError(t, err)
	assert.Equal(t, []string{titleWanted}, titlesOf(narrowed))

	// a combination nothing satisfies is an empty result, not an error.
	combined.Keyword = ""
	combined.FetchStatus = service.FetchStatusUnfetched

	none, err := svc.AllSources(combined)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestLegalParseTypesIsTheFilterVocabulary(t *testing.T) {
	svc := newService(t)

	types := feed.LegalParseTypes()
	assert.Equal(t, []string{feed.ParseTypeFeed, feed.ParseTypeRegex, feed.ParseTypeJSON}, types)

	// every offered type is a value the query accepts, so the sidebar can build
	// its options from this set alone.
	for _, parseType := range types {
		_, err := svc.AllSources(service.SourceQuery{ParseType: parseType})
		require.NoError(t, err, "parse type %q", parseType)
	}

	types[0] = "mutated"

	assert.Equal(t, feed.ParseTypeFeed, feed.LegalParseTypes()[0], "the caller cannot mutate the shared set")
}
