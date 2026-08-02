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

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// Category names the cases below reuse.
const (
	techName = "tech"
	newsName = "news"
	// the seeded category keeps its shipped chinese name.
	defaultName = "\u672a\u5206\u7c7b"
)

func TestCategoryTreeCRUD(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	seeded, err := app.ListCategories()
	require.NoError(t, err)
	require.Len(t, seeded, 1)
	assert.True(t, seeded[0].IsDefault)
	assert.Zero(t, seeded[0].SourceCount)

	tech, err := app.CreateCategory(&SaveCategoryRequest{Name: techName, Sort: 5})
	require.NoError(t, err)
	assert.NotZero(t, tech.ID)

	news, err := app.CreateCategory(&SaveCategoryRequest{Name: newsName, Sort: -1})
	require.NoError(t, err)

	listed, err := app.ListCategories()
	require.NoError(t, err)
	require.Len(t, listed, 3)
	assert.Equal(t, []string{newsName, defaultName, techName}, categoryNames(listed))

	// editing the sort value alone re-orders the tree.
	_, err = app.UpdateCategory(&SaveCategoryRequest{ID: news.ID, Name: newsName, Sort: 99})
	require.NoError(t, err)

	listed, err = app.ListCategories()
	require.NoError(t, err)
	assert.Equal(t, []string{defaultName, techName, newsName}, categoryNames(listed))

	_, err = app.CreateCategory(nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = app.UpdateCategory(&SaveCategoryRequest{ID: tech.ID, Name: ""})
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func TestDeleteCategoryReportsUseBeforeMoving(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	tech, err := app.CreateCategory(&SaveCategoryRequest{Name: techName})
	require.NoError(t, err)

	req := sampleRequest()
	req.CategoryID = tech.ID

	source, err := app.CreateSource(req)
	require.NoError(t, err)

	// the plain delete is refused, and the refusal arrives as data, not as a failure.
	refused, err := app.DeleteCategory(tech.ID, 0)
	require.NoError(t, err)
	assert.False(t, refused.Deleted)
	assert.Equal(t, int64(1), refused.InUseCount)

	// naming the target moves the subscription and then deletes.
	done, err := app.DeleteCategory(tech.ID, feed.DefaultCategoryID)
	require.NoError(t, err)
	assert.True(t, done.Deleted)
	assert.Equal(t, int64(1), done.Moved)

	sources, err := app.ListSources(0)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, source.ID, sources[0].ID)
	assert.Equal(t, int64(feed.DefaultCategoryID), sources[0].CategoryID)

	// the default category itself is never deletable.
	_, err = app.DeleteCategory(feed.DefaultCategoryID, 0)
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func TestListSourcesFiltersByCategory(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	tech, err := app.CreateCategory(&SaveCategoryRequest{Name: techName})
	require.NoError(t, err)

	inTech := sampleRequest()
	inTech.CategoryID = tech.ID
	inTech.Title = "in tech"
	_, err = app.CreateSource(inTech)
	require.NoError(t, err)

	elsewhere := sampleRequest()
	elsewhere.Title = "default"
	_, err = app.CreateSource(elsewhere)
	require.NoError(t, err)

	all, err := app.ListSources(0)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	filtered, err := app.ListSources(tech.ID)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "in tech", filtered[0].Title)

	// a category without subscriptions is an empty list, not an error.
	empty, err := app.CreateCategory(&SaveCategoryRequest{Name: "empty"})
	require.NoError(t, err)

	none, err := app.ListSources(empty.ID)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestDailyBindings(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	empty, err := app.DailyIndex()
	require.NoError(t, err)
	assert.Empty(t, empty)

	writeDailyFile(t, app, "2026-01-02", "# daily\n\n- article, https://example.com/one\n")
	writeDailyFile(t, app, "2026-02-03", "# february\n")

	years, err := app.DailyIndex()
	require.NoError(t, err)
	require.Len(t, years, 1)
	require.Len(t, years[0].Months, 2)
	assert.Equal(t, "2026-02", years[0].Months[0].Month)
	assert.Equal(t, "2026-01-02", years[0].Months[1].Days[0].Date)

	content, err := app.DailyContent("2026-01-02")
	require.NoError(t, err)
	assert.Contains(t, content, "https://example.com/one")

	_, err = app.DailyContent("2026-01-03")
	require.ErrorIs(t, err, service.ErrNotFound)

	_, err = app.DailyContent("../../secret")
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func TestArticleLibraryPaging(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	tech, err := app.CreateCategory(&SaveCategoryRequest{Name: techName})
	require.NoError(t, err)

	req := sampleRequest()
	req.CategoryID = tech.ID
	req.Title = "tech source"

	source, err := app.CreateSource(req)
	require.NoError(t, err)

	const total = 7

	for i := range total {
		require.NoError(t, app.svc.CreateArticle(&feed.Article{
			URL:      "https://example.com/" + string(rune('a'+i)),
			Title:    "article",
			SourceID: source.ID,
		}))
	}

	seen := map[int64]bool{}
	cursors := []int64{0}
	cursor := int64(0)

	for range 5 {
		page, pageErr := app.ListArticles(&ArticleQueryRequest{Limit: 3, Before: cursor})
		require.NoError(t, pageErr)

		for _, item := range page.Items {
			require.False(t, seen[item.ID], "a cursor page repeated article %d", item.ID)
			seen[item.ID] = true
			assert.Equal(t, "tech source", item.SourceTitle)
			assert.Equal(t, tech.ID, item.CategoryID)
			assert.Zero(t, item.InformedAt, "an article never delivered has no inform time")
		}

		if !page.HasMore {
			break
		}

		cursor = page.NextCursor
		cursors = append(cursors, cursor)
	}

	assert.Len(t, seen, total)

	// paging back through the remembered cursors lands on the same rows again.
	first, err := app.ListArticles(&ArticleQueryRequest{Limit: 3, Before: cursors[0]})
	require.NoError(t, err)
	require.Len(t, first.Items, 3)
}

func TestArticleLibraryFiltersAndRejectsABadRequest(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	// a filter that matches nothing is an empty page, not a failure.
	other, err := app.ListArticles(&ArticleQueryRequest{Limit: 3, CategoryID: feed.DefaultCategoryID})
	require.NoError(t, err)
	assert.Empty(t, other.Items)
	assert.False(t, other.HasMore)

	_, err = app.ListArticles(nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func TestRebuildHistoryIndexBinding(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	require.NoError(t, app.svc.CreateArticle(&feed.Article{URL: "https://example.com/one", Title: "one"}))
	writeDailyFile(t, app, "2026-01-02", "- one, https://example.com/one\n- unknown, https://example.com/ghost\n")

	result, err := app.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Equal(t, 1, result.Days)
	assert.Equal(t, 2, result.Links)
	assert.Equal(t, 1, result.Filled)
	assert.Equal(t, 1, result.SkippedUnmatched)
	assert.Equal(t, 1, result.Skipped)
	assert.Zero(t, result.Failed)
	assert.Empty(t, result.Errors)

	// running it again changes nothing and says so.
	again, err := app.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Zero(t, again.Filled)
	assert.Equal(t, 1, again.SkippedAlreadyIndexed)

	page, err := app.ListArticles(&ArticleQueryRequest{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.NotZero(t, page.Items[0].InformedAt)
}

func TestConfigBindingsRoundTrip(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	view, err := app.ReadConfig()
	require.NoError(t, err)
	assert.False(t, view.Exists)
	assert.Equal(t, view.Defaults, view.Feed, "a missing file offers the documented defaults")
	assert.Empty(t, view.PreservedKeys)

	require.NoError(t, app.SaveConfig(&FeedConfigDTO{
		MaxInformFeedSize: 15,
		FeedExpireDays:    30,
		SameSiteMaxCount:  2,
		MaxFetchNum:       0,
	}))

	view, err = app.ReadConfig()
	require.NoError(t, err)
	assert.True(t, view.Exists)
	assert.Equal(t, 15, view.Feed.MaxInformFeedSize)
	assert.Equal(t, 30, view.Feed.FeedExpireDays)

	// the CLI path reads the very same file.
	fileConfig, err := app.svc.ReadFileConfig()
	require.NoError(t, err)
	require.NotNil(t, fileConfig.Feed)
	assert.Equal(t, 15, fileConfig.Feed.MaxInformFeedSize)

	// an invalid value is refused and the stored file keeps its previous content.
	require.ErrorIs(t, app.SaveConfig(&FeedConfigDTO{}), service.ErrInvalidArgument)
	require.ErrorIs(t, app.SaveConfig(nil), service.ErrInvalidArgument)

	view, err = app.ReadConfig()
	require.NoError(t, err)
	assert.Equal(t, 15, view.Feed.MaxInformFeedSize)
}

func TestScheduleBindingsRoundTrip(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	view, err := app.ReadConfig()
	require.NoError(t, err)
	assert.Equal(t, view.ScheduleDefaults, view.Schedule, "a missing file offers the documented defaults")
	assert.False(t, view.Schedule.Enabled)
	assert.Equal(t, "10:00", view.Schedule.Time)

	require.NoError(t, app.SaveSchedule(&ScheduleDTO{Enabled: true, Time: "21:45"}))

	view, err = app.ReadConfig()
	require.NoError(t, err)
	require.NotNil(t, view.Schedule)
	assert.True(t, view.Schedule.Enabled)
	assert.Equal(t, "21:45", view.Schedule.Time)

	// the desktop scheduler reads the very same file through its own path.
	schedule, err := app.svc.ReadScheduleConfig()
	require.NoError(t, err)
	assert.True(t, schedule.Enabled)
	assert.Equal(t, "21:45", schedule.Time)

	// an invalid time is refused and the stored file keeps its previous content.
	require.ErrorIs(t, app.SaveSchedule(&ScheduleDTO{Enabled: true, Time: "25:00"}), service.ErrInvalidArgument)
	require.ErrorIs(t, app.SaveSchedule(nil), service.ErrInvalidArgument)

	view, err = app.ReadConfig()
	require.NoError(t, err)
	assert.Equal(t, "21:45", view.Schedule.Time)

	// a schedule save leaves the feed section alone, and the other way around.
	require.NoError(t, app.SaveConfig(&FeedConfigDTO{
		MaxInformFeedSize: 12,
		FeedExpireDays:    60,
		SameSiteMaxCount:  4,
		MaxFetchNum:       1,
	}))

	view, err = app.ReadConfig()
	require.NoError(t, err)
	assert.Equal(t, 12, view.Feed.MaxInformFeedSize)
	assert.Equal(t, "21:45", view.Schedule.Time)
}

func TestWebhookBindingReturnsTheStoredAddress(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	view, err := app.ReadSecrets()
	require.NoError(t, err)
	assert.False(t, view.WebhookConfigured)

	const webhook = "https://open.feishu.cn/open-apis/bot/v2/hook/0000-secret-token"

	require.NoError(t, app.SaveWebhook(webhook))

	view, err = app.ReadSecrets()
	require.NoError(t, err)
	assert.True(t, view.WebhookConfigured)
	assert.Equal(t, webhook, view.Webhook)

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(view.Path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	require.NoError(t, app.SaveWebhook(""))

	view, err = app.ReadSecrets()
	require.NoError(t, err)
	assert.False(t, view.WebhookConfigured)
}

func TestNewBindingsReportAStartupFailure(t *testing.T) {
	t.Parallel()

	app := newAppWithHome(filepath.Join(t.TempDir(), "missing", "nested"))
	require.NotEmpty(t, app.StartupError())

	_, err := app.ListCategories()
	require.ErrorIs(t, err, ErrNotReady)

	_, err = app.DailyIndex()
	require.ErrorIs(t, err, ErrNotReady)

	_, err = app.ListArticles(&ArticleQueryRequest{})
	require.ErrorIs(t, err, ErrNotReady)

	_, err = app.ReadConfig()
	require.ErrorIs(t, err, ErrNotReady)

	_, err = app.RebuildHistoryIndex()
	require.ErrorIs(t, err, ErrNotReady)

	require.ErrorIs(t, app.SaveWebhook("x"), ErrNotReady)

	require.ErrorIs(t, app.SaveSchedule(&ScheduleDTO{Enabled: true, Time: "10:00"}), ErrNotReady)

	_, err = app.TriggerNow()
	require.ErrorIs(t, err, ErrNotReady)
}

func categoryNames(categories []*CategoryDTO) []string {
	names := make([]string, 0, len(categories))
	for _, category := range categories {
		names = append(names, category.Name)
	}

	return names
}

func writeDailyFile(t *testing.T, app *App, date, content string) {
	t.Helper()

	dir := filepath.Join(app.homeDir, "data", date[:4])
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, date+".md"), []byte(content), 0o600))
}
