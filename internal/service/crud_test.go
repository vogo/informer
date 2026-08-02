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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/service"
)

func TestNewRejectsEmptyHomeDir(t *testing.T) {
	_, err := service.New("")
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func TestSourceCRUD(t *testing.T) {
	svc := newService(t)

	source := &feed.Source{Title: "blog", URL: "https://example.com/atom.xml"}
	require.NoError(t, svc.CreateSource(source))
	require.NotZero(t, source.ID)

	// a new source is enabled and lands in the default category.
	assert.True(t, source.Enabled)
	assert.Equal(t, int64(feed.DefaultCategoryID), source.CategoryID)

	stored, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	assert.Equal(t, "blog", stored.Title)
	assert.True(t, stored.Enabled)

	stored.Title = "renamed blog"
	stored.Weight = 80
	require.NoError(t, svc.UpdateSource(stored))

	stored, err = svc.GetSource(source.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed blog", stored.Title)
	assert.Equal(t, int64(80), stored.Weight)

	require.NoError(t, svc.UpdateSourceColumn(source.ID, "max_fetch_num", 3))

	stored, err = svc.GetSource(source.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, stored.MaxFetchNum)

	require.NoError(t, svc.SetSourceEnabled(source.ID, false))

	stored, err = svc.GetSource(source.ID)
	require.NoError(t, err)
	assert.False(t, stored.Enabled)

	require.NoError(t, svc.DeleteSource(source.ID))

	_, err = svc.GetSource(source.ID)
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestSourceErrorPropagation(t *testing.T) {
	svc := newService(t)

	require.ErrorIs(t, svc.CreateSource(nil), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.CreateSource(&feed.Source{Title: "no url"}), service.ErrInvalidArgument)
	require.ErrorIs(t,
		svc.CreateSource(&feed.Source{URL: "https://example.com", ParseType: "xml"}),
		service.ErrInvalidArgument)
	require.ErrorIs(t,
		svc.CreateSource(&feed.Source{URL: "https://example.com", CategoryID: 999}),
		service.ErrNotFound)

	require.ErrorIs(t, svc.UpdateSource(&feed.Source{}), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.UpdateSource(&feed.Source{ID: 404, URL: "x"}), service.ErrNotFound)
	require.ErrorIs(t, svc.UpdateSourceColumn(1, "", "x"), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.DeleteSource(404), service.ErrNotFound)

	_, err := svc.GetSource(404)
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestListSourcesPagingIsStable(t *testing.T) {
	svc := newService(t)

	for i := range 7 {
		require.NoError(t, svc.CreateSource(&feed.Source{
			Title: fmt.Sprintf("source-%d", i),
			URL:   fmt.Sprintf("https://example.com/%d", i),
		}))
	}

	first, err := svc.ListSources(service.SourceQuery{}, service.PageRequest{Offset: 0, Limit: 3})
	require.NoError(t, err)
	require.Len(t, first.Items, 3)
	assert.Equal(t, int64(7), first.Total)
	assert.True(t, first.HasMore)
	assert.Equal(t, "source-0", first.Items[0].Title)
	assert.Equal(t, "source-2", first.Items[2].Title)

	second, err := svc.ListSources(service.SourceQuery{}, service.PageRequest{Offset: 3, Limit: 3})
	require.NoError(t, err)
	require.Len(t, second.Items, 3)
	assert.Equal(t, "source-3", second.Items[0].Title)

	last, err := svc.ListSources(service.SourceQuery{}, service.PageRequest{Offset: 6, Limit: 3})
	require.NoError(t, err)
	require.Len(t, last.Items, 1)
	assert.False(t, last.HasMore)

	// repeating a page returns exactly the same slice.
	again, err := svc.ListSources(service.SourceQuery{}, service.PageRequest{Offset: 0, Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, first.Items[0].ID, again.Items[0].ID)
	assert.Equal(t, first.Items[2].ID, again.Items[2].ID)
}

func TestListSourcesEmptyResult(t *testing.T) {
	svc := newService(t)

	page, err := svc.ListSources(service.SourceQuery{}, service.PageRequest{})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
	assert.NotNil(t, page.Items, "an empty page carries an empty slice, not nil")
	assert.Zero(t, page.Total)
	assert.False(t, page.HasMore)
	assert.Equal(t, service.DefaultPageLimit, page.Limit)

	// an offset beyond the end is not an error either.
	page, err = svc.ListSources(service.SourceQuery{}, service.PageRequest{Offset: 100})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestListSourcesFilters(t *testing.T) {
	svc := newService(t)

	other := &feed.Category{Name: "技术", Sort: 1}
	require.NoError(t, svc.CreateCategory(other))

	require.NoError(t, svc.CreateSource(&feed.Source{Title: "alpha", URL: "https://a.example.com"}))
	require.NoError(t, svc.CreateSource(&feed.Source{Title: "beta", URL: "https://b.example.com", CategoryID: other.ID}))

	disabled := &feed.Source{Title: "gamma", URL: "https://c.example.com"}
	require.NoError(t, svc.CreateSource(disabled))
	require.NoError(t, svc.SetSourceEnabled(disabled.ID, false))

	page, err := svc.ListSources(service.SourceQuery{CategoryID: other.ID}, service.PageRequest{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "beta", page.Items[0].Title)

	enabled := true
	page, err = svc.ListSources(service.SourceQuery{Enabled: &enabled}, service.PageRequest{})
	require.NoError(t, err)
	assert.Len(t, page.Items, 2)

	page, err = svc.ListSources(service.SourceQuery{Keyword: "amm"}, service.PageRequest{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "gamma", page.Items[0].Title)
}

func TestCategoryCRUD(t *testing.T) {
	svc := newService(t)

	seeded, err := svc.GetCategory(feed.DefaultCategoryID)
	require.NoError(t, err)
	assert.Equal(t, feed.DefaultCategoryName, seeded.Name)

	category := &feed.Category{Name: "技术", Sort: 5}
	require.NoError(t, svc.CreateCategory(category))

	category.Name = "工程"
	require.NoError(t, svc.UpdateCategory(category))

	stored, err := svc.GetCategory(category.ID)
	require.NoError(t, err)
	assert.Equal(t, "工程", stored.Name)

	require.NoError(t, svc.DeleteCategory(category.ID))

	_, err = svc.GetCategory(category.ID)
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestCategoryDeleteRules(t *testing.T) {
	svc := newService(t)

	require.ErrorIs(t, svc.DeleteCategory(feed.DefaultCategoryID), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.DeleteCategory(404), service.ErrNotFound)

	category := &feed.Category{Name: "技术"}
	require.NoError(t, svc.CreateCategory(category))
	require.NoError(t, svc.CreateSource(&feed.Source{
		Title:      "member",
		URL:        "https://example.com/x",
		CategoryID: category.ID,
	}))

	// a category still holding sources is never removed, so no source is left dangling.
	require.ErrorIs(t, svc.DeleteCategory(category.ID), service.ErrCategoryInUse)

	_, err := svc.GetCategory(category.ID)
	require.NoError(t, err)
}

func TestListCategoriesOrderAndPaging(t *testing.T) {
	svc := newService(t)

	require.NoError(t, svc.CreateCategory(&feed.Category{Name: "last", Sort: 9}))
	require.NoError(t, svc.CreateCategory(&feed.Category{Name: "first", Sort: -1}))
	require.NoError(t, svc.CreateCategory(&feed.Category{Name: "same-sort", Sort: 0}))

	all, err := svc.AllCategories()
	require.NoError(t, err)
	require.Len(t, all, 4)
	assert.Equal(t, "first", all[0].Name)
	// the seeded category and "same-sort" share sort 0, so the id decides.
	assert.Equal(t, feed.DefaultCategoryName, all[1].Name)
	assert.Equal(t, "same-sort", all[2].Name)
	assert.Equal(t, "last", all[3].Name)

	page, err := svc.ListCategories(service.PageRequest{Offset: 1, Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, int64(4), page.Total)
	assert.True(t, page.HasMore)
	assert.Equal(t, feed.DefaultCategoryName, page.Items[0].Name)
}

func TestArticleCRUD(t *testing.T) {
	svc := newService(t)

	article := &feed.Article{URL: "https://example.com/a", Title: "an article"}
	require.NoError(t, svc.CreateArticle(article))
	require.NotZero(t, article.ID)

	stored, err := svc.GetArticle(article.ID)
	require.NoError(t, err)
	assert.Equal(t, "an article", stored.Title)
	assert.Nil(t, stored.FetchedAt)
	assert.Nil(t, stored.InformedAt)

	stored.Title = "renamed"
	require.NoError(t, svc.UpdateArticle(stored))

	stored, err = svc.GetArticle(article.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", stored.Title)

	require.NoError(t, svc.DeleteArticle(article.ID))

	_, err = svc.GetArticle(article.ID)
	require.ErrorIs(t, err, service.ErrNotFound)

	require.ErrorIs(t, svc.CreateArticle(nil), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.CreateArticle(&feed.Article{Title: "no url"}), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.UpdateArticle(&feed.Article{}), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.DeleteArticle(404), service.ErrNotFound)
}

func TestListArticlesFiltersAndPaging(t *testing.T) {
	svc := newService(t)

	category := &feed.Category{Name: "技术"}
	require.NoError(t, svc.CreateCategory(category))

	plain := &feed.Source{Title: "plain", URL: "https://example.com/p"}
	require.NoError(t, svc.CreateSource(plain))

	grouped := &feed.Source{Title: "grouped", URL: "https://example.com/g", CategoryID: category.ID}
	require.NoError(t, svc.CreateSource(grouped))

	for i := range 5 {
		require.NoError(t, svc.CreateArticle(&feed.Article{
			URL:      fmt.Sprintf("https://example.com/plain/%d", i),
			Title:    fmt.Sprintf("plain-%d", i),
			SourceID: plain.ID,
			Informed: i%2 == 0,
		}))
	}

	require.NoError(t, svc.CreateArticle(&feed.Article{
		URL:      "https://example.com/grouped/0",
		Title:    "grouped-0",
		SourceID: grouped.ID,
	}))

	page, err := svc.ListArticles(service.ArticleQuery{SourceID: plain.ID}, service.PageRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, int64(5), page.Total)
	assert.True(t, page.HasMore)
	assert.Equal(t, "plain-4", page.Items[0].Title, "articles are listed newest first")

	page, err = svc.ListArticles(service.ArticleQuery{CategoryID: category.ID}, service.PageRequest{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "grouped-0", page.Items[0].Title)

	informed := true
	page, err = svc.ListArticles(service.ArticleQuery{Informed: &informed}, service.PageRequest{})
	require.NoError(t, err)
	assert.Len(t, page.Items, 3)

	page, err = svc.ListArticles(service.ArticleQuery{Keyword: "grouped"}, service.PageRequest{})
	require.NoError(t, err)
	assert.Len(t, page.Items, 1)

	page, err = svc.ListArticles(service.ArticleQuery{SourceID: 999}, service.PageRequest{})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestConfigCRUDAndPaging(t *testing.T) {
	svc := newService(t)

	config := &feed.Config{MaxInformFeedSize: 5, FeedExpireDays: 30, SameSiteMaxCount: 2, MaxFetchNum: 4}
	require.NoError(t, svc.CreateConfig(config))
	require.NotZero(t, config.ID)

	stored, err := svc.GetConfig(config.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, stored.MaxInformFeedSize)

	stored.MaxInformFeedSize = 12
	require.NoError(t, svc.UpdateConfig(stored))

	stored, err = svc.GetConfig(config.ID)
	require.NoError(t, err)
	assert.Equal(t, 12, stored.MaxInformFeedSize)

	page, err := svc.ListConfigs(service.PageRequest{})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, int64(1), page.Total)

	require.NoError(t, svc.DeleteConfig(config.ID))

	_, err = svc.GetConfig(config.ID)
	require.ErrorIs(t, err, service.ErrNotFound)

	require.ErrorIs(t, svc.CreateConfig(nil), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.UpdateConfig(&feed.Config{}), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.DeleteConfig(404), service.ErrNotFound)

	page, err = svc.ListConfigs(service.PageRequest{})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestPageLimitBounds(t *testing.T) {
	svc := newService(t)

	page, err := svc.ListSources(service.SourceQuery{}, service.PageRequest{Offset: -5, Limit: -1})
	require.NoError(t, err)
	assert.Zero(t, page.Offset)
	assert.Equal(t, service.DefaultPageLimit, page.Limit)

	page, err = svc.ListSources(service.SourceQuery{}, service.PageRequest{Limit: 10_000})
	require.NoError(t, err)
	assert.Equal(t, service.MaxPageLimit, page.Limit)
}

func TestEffectiveFeedConfigPrefersFile(t *testing.T) {
	svc := newService(t)

	fileConfig, raw, err := svc.ReadFileConfig()
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.NotNil(t, fileConfig.Feed)

	// informer.json stays authoritative for an existing installation.
	require.NoError(t, svc.CreateConfig(&feed.Config{MaxInformFeedSize: 99}))
	assert.Equal(t, 10, svc.EffectiveFeedConfig(fileConfig).MaxInformFeedSize)

	// only a file without a feed section falls back to the stored record.
	stored := svc.EffectiveFeedConfig(&inform.Config{})
	require.NotNil(t, stored)
	assert.Equal(t, 99, stored.MaxInformFeedSize)

	// and with neither, feeds are simply skipped.
	empty := newService(t)
	assert.Nil(t, empty.EffectiveFeedConfig(&inform.Config{}))
}

func TestReadFileConfigErrors(t *testing.T) {
	dir := t.TempDir()

	svc, err := service.New(dir)
	require.NoError(t, err)

	_, _, err = svc.ReadFileConfig()
	require.ErrorContains(t, err, "read config file")

	require.NoError(t, writeRaw(dir, "not json"))

	_, _, err = svc.ReadFileConfig()
	require.ErrorContains(t, err, "parse config file")
}
