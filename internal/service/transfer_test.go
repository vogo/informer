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

// Titles and expressions the cases below reuse. agentTitle identifies the agent
// subscription: it has no url at all, so its title is the only thing an import
// can match it by.
const (
	agentTitle = "agent watcher"
	blogTitle  = "blog"
	firstTitle = "first"
	titleExp   = "$2"
	urlExp     = "$1"
)

// exportedFixture fills one service with a subscription of every shape the
// export has to carry, and returns its export document.
func exportedFixture(t *testing.T) *service.SourceExportDoc {
	t.Helper()

	svc := newService(t)

	require.NoError(t, svc.CreateCategory(&feed.Category{Name: techCategory, Sort: 3}))

	categories, err := svc.AllCategories()
	require.NoError(t, err)

	var techID int64

	for _, category := range categories {
		if category.Name == techCategory {
			techID = category.ID
		}
	}

	require.NotZero(t, techID)

	blog := &feed.Source{
		Title:       blogTitle,
		URL:         feedURLA,
		ParseType:   feed.ParseTypeFeed,
		CategoryID:  techID,
		Weight:      42,
		MaxFetchNum: 7,
	}
	require.NoError(t, svc.CreateSource(blog))

	regex := &feed.Source{
		Title:      "regex site",
		URL:        feedURLB,
		ParseType:  feed.ParseTypeRegex,
		Regex:      `href="([^"]+)">([^<]+)<`,
		TitleExp:   titleExp,
		URLExp:     urlExp,
		Redirect:   true,
		CategoryID: techID,
	}
	require.NoError(t, svc.CreateSource(regex))

	agentSource := &feed.Source{
		Title:       agentTitle,
		ParseType:   feed.ParseTypeAgent,
		AgentPrompt: agentPrompt,
	}
	require.NoError(t, svc.CreateSource(agentSource))
	// a disabled subscription has to come back disabled.
	require.NoError(t, svc.SetSourceEnabled(agentSource.ID, false))

	doc, err := svc.ExportSources()
	require.NoError(t, err)
	require.Len(t, doc.Sources, 3)

	return doc
}

// sourceByTitle reads one stored subscription by title.
func sourceByTitle(t *testing.T, svc *service.Service, title string) *feed.Source {
	t.Helper()

	sources, err := svc.AllSources(service.SourceQuery{})
	require.NoError(t, err)

	for _, source := range sources {
		if source.Title == title {
			return source
		}
	}

	require.FailNowf(t, "subscription not found", "title %q", title)

	return nil
}

func TestExportSourcesCarriesEveryConfigurationField(t *testing.T) {
	doc := exportedFixture(t)

	assert.Equal(t, service.SourceExportVersion, doc.Version)
	assert.NotEmpty(t, doc.ExportedAt)

	blog := doc.Sources[0]
	assert.Equal(t, blogTitle, blog.Title)
	assert.Equal(t, feedURLA, blog.URL)
	assert.Equal(t, feed.ParseTypeFeed, blog.ParseType)
	// the category travels by name, never by the id of this installation.
	assert.Equal(t, techCategory, blog.Category)
	assert.Equal(t, int64(42), blog.Weight)
	assert.Equal(t, 7, blog.MaxFetchNum)
	assert.True(t, blog.Enabled)

	regex := doc.Sources[1]
	assert.Equal(t, `href="([^"]+)">([^<]+)<`, regex.Regex)
	assert.Equal(t, titleExp, regex.TitleExp)
	assert.Equal(t, urlExp, regex.URLExp)
	assert.True(t, regex.Redirect)

	agentEntry := doc.Sources[2]
	assert.Empty(t, agentEntry.URL)
	assert.Equal(t, agentPrompt, agentEntry.AgentPrompt)
	assert.False(t, agentEntry.Enabled)
	// the default category is named, and an unresolvable one would be empty.
	assert.Equal(t, feed.DefaultCategoryName, agentEntry.Category)
}

func TestImportSourcesRestoresAnExportOnAFreshInstallation(t *testing.T) {
	doc := exportedFixture(t)

	target := newService(t)

	result, err := target.ImportSources(doc)
	require.NoError(t, err)
	assert.Equal(t, &service.ImportSourcesResult{Total: 3, Created: 3}, result)

	restored, err := target.AllSources(service.SourceQuery{})
	require.NoError(t, err)
	require.Len(t, restored, 3)

	// the missing category was created on the way in, and both subscriptions
	// that named it point at the very same new category.
	categories, err := target.AllCategories()
	require.NoError(t, err)
	require.Len(t, categories, 2)

	blog := sourceByTitle(t, target, blogTitle)
	assert.Equal(t, feedURLA, blog.URL)
	assert.Equal(t, int64(42), blog.Weight)
	assert.NotEqual(t, int64(feed.DefaultCategoryID), blog.CategoryID)
	assert.Equal(t, blog.CategoryID, sourceByTitle(t, target, "regex site").CategoryID)

	// the disabled agent subscription comes back disabled, not enabled by the
	// create rule.
	agentSource := sourceByTitle(t, target, agentTitle)
	assert.False(t, agentSource.Enabled)
	assert.Equal(t, agentPrompt, agentSource.AgentPrompt)

	// re-importing the same file changes nothing and creates nothing.
	result, err = target.ImportSources(doc)
	require.NoError(t, err)
	assert.Equal(t, &service.ImportSourcesResult{Total: 3, Updated: 3}, result)

	restored, err = target.AllSources(service.SourceQuery{})
	require.NoError(t, err)
	assert.Len(t, restored, 3)
}

func TestImportSourcesOverwritesTheMatchedSubscriptionAndAppendsTheRest(t *testing.T) {
	svc := newService(t)

	stored := &feed.Source{Title: "old title", URL: feedURLA, ParseType: feed.ParseTypeFeed, Weight: 1}
	require.NoError(t, svc.CreateSource(stored))
	// the fetch health of this machine has to survive the import.
	stored.Status = feed.StatusError
	stored.ErrorInfo = "boom"
	require.NoError(t, svc.UpdateSource(stored))

	doc := &service.SourceExportDoc{
		Version: service.SourceExportVersion,
		Sources: []*service.SourceExport{
			// same url, everything else different: an overwrite, not a second row.
			{Title: "new title", URL: feedURLA, ParseType: feed.ParseTypeFeed, Weight: 9, Category: techCategory, Enabled: true},
			{Title: "fresh", URL: feedURLB, ParseType: feed.ParseTypeFeed, Enabled: true},
		},
	}

	result, err := svc.ImportSources(doc)
	require.NoError(t, err)
	assert.Equal(t, &service.ImportSourcesResult{Total: 2, Created: 1, Updated: 1}, result)

	sources, err := svc.AllSources(service.SourceQuery{})
	require.NoError(t, err)
	require.Len(t, sources, 2)

	updated := sourceByTitle(t, svc, "new title")
	assert.Equal(t, stored.ID, updated.ID)
	assert.Equal(t, int64(9), updated.Weight)
	assert.Equal(t, feed.StatusError, updated.Status)
	assert.Equal(t, "boom", updated.ErrorInfo)
}

func TestImportSourcesMatchesAUrllessSubscriptionByTitle(t *testing.T) {
	svc := newService(t)

	stored := &feed.Source{Title: agentTitle, ParseType: feed.ParseTypeAgent, AgentPrompt: "old prompt"}
	require.NoError(t, svc.CreateSource(stored))

	doc := &service.SourceExportDoc{
		Sources: []*service.SourceExport{
			{Title: agentTitle, ParseType: feed.ParseTypeAgent, AgentPrompt: "new prompt", Enabled: true},
		},
	}

	result, err := svc.ImportSources(doc)
	require.NoError(t, err)
	assert.Equal(t, &service.ImportSourcesResult{Total: 1, Updated: 1}, result)

	assert.Equal(t, "new prompt", sourceByTitle(t, svc, agentTitle).AgentPrompt)
}

func TestImportSourcesReportsOneUnusableEntryAndKeepsGoing(t *testing.T) {
	svc := newService(t)

	doc := &service.SourceExportDoc{
		Sources: []*service.SourceExport{
			// neither url nor title: nothing to identify or fetch.
			{Enabled: true},
			// an agent entry without a prompt is refused by the shared validation.
			{Title: "no prompt", ParseType: feed.ParseTypeAgent, Enabled: true},
			{Title: "good", URL: feedURLA, ParseType: feed.ParseTypeFeed, Enabled: true},
		},
	}

	result, err := svc.ImportSources(doc)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 2, result.Failed)
	require.Len(t, result.Errors, 2)
	assert.Contains(t, result.Errors[0], "entry 1")
	assert.Contains(t, result.Errors[1], "no prompt")

	sources, err := svc.AllSources(service.SourceQuery{})
	require.NoError(t, err)
	assert.Len(t, sources, 1)
}

func TestImportSourcesAppliesADuplicatedEntryOnce(t *testing.T) {
	svc := newService(t)

	doc := &service.SourceExportDoc{
		Sources: []*service.SourceExport{
			{Title: firstTitle, URL: feedURLA, ParseType: feed.ParseTypeFeed, Enabled: true},
			{Title: "second", URL: feedURLA, ParseType: feed.ParseTypeFeed, Enabled: true},
		},
	}

	result, err := svc.ImportSources(doc)
	require.NoError(t, err)
	assert.Equal(t, &service.ImportSourcesResult{Total: 2, Created: 1, Updated: 1}, result)

	sources, err := svc.AllSources(service.SourceQuery{})
	require.NoError(t, err)
	require.Len(t, sources, 1)
	// the later entry of the same url wins, as it does on a second import.
	assert.Equal(t, "second", sources[0].Title)
}

func TestImportSourcesRefusesANewerExportVersion(t *testing.T) {
	svc := newService(t)

	_, err := svc.ImportSources(&service.SourceExportDoc{Version: service.SourceExportVersion + 1})
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = svc.ImportSources(nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func TestSourceExportRoundTripsThroughJSON(t *testing.T) {
	doc := exportedFixture(t)

	data, err := service.MarshalSourceExport(doc)
	require.NoError(t, err)

	target := newService(t)

	result, err := target.ImportSourcesJSON(data)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Created)

	again, err := target.ExportSources()
	require.NoError(t, err)
	assert.Equal(t, doc.Sources, again.Sources)
}

func TestParseSourceExportAcceptsABareArrayAndRefusesGarbage(t *testing.T) {
	doc, err := service.ParseSourceExport([]byte(`[{"title":"a","url":"https://example.com/a","enabled":true}]`))
	require.NoError(t, err)
	require.Len(t, doc.Sources, 1)
	assert.Equal(t, "a", doc.Sources[0].Title)

	_, err = service.ParseSourceExport([]byte("   "))
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = service.ParseSourceExport([]byte("not json"))
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}
