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

// Category names and urls reused across the cases below.
const (
	techCategory = "tech"
	feedURLA     = "https://example.com/feed-a"
	feedURLB     = "https://example.com/feed-b"
)

func TestCategoriesAreOrderedBySortThenID(t *testing.T) {
	svc := newService(t)

	const (
		last  = "zz last"
		first = "aa first"
		tieA  = "tie a"
		tieB  = "tie b"
	)

	lastCategory := &feed.Category{Name: last, Sort: 30}
	firstCategory := &feed.Category{Name: first, Sort: -10}
	tieACategory := &feed.Category{Name: tieA, Sort: 5}
	tieBCategory := &feed.Category{Name: tieB, Sort: 5}

	for _, category := range []*feed.Category{lastCategory, firstCategory, tieACategory, tieBCategory} {
		require.NoError(t, svc.CreateCategory(category))
	}

	names := categoryNames(t, svc)
	assert.Equal(t, []string{first, feed.DefaultCategoryName, tieA, tieB, last}, names)

	// editing the sort value alone re-orders the tree and survives a reload.
	tieBCategory.Sort = -20
	require.NoError(t, svc.UpdateCategory(tieBCategory))

	reopened, err := service.New(svc.HomeDir())
	require.NoError(t, err)
	assert.Equal(t, []string{tieB, first, feed.DefaultCategoryName, tieA, last}, categoryNames(t, reopened))
}

func TestDeleteCategoryRefusesTheDefaultAndCategoriesInUse(t *testing.T) {
	svc := newService(t)

	require.ErrorIs(t, svc.DeleteCategory(feed.DefaultCategoryID), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.DeleteCategory(9999), service.ErrNotFound)

	category := &feed.Category{Name: techCategory}
	require.NoError(t, svc.CreateCategory(category))

	source := &feed.Source{URL: "https://example.com/feed", CategoryID: category.ID}
	require.NoError(t, svc.CreateSource(source))

	require.ErrorIs(t, svc.DeleteCategory(category.ID), service.ErrCategoryInUse)

	// the refusal is total: the category and its source are both still there.
	_, err := svc.GetCategory(category.ID)
	require.NoError(t, err)
	assert.Equal(t, category.ID, reloadSource(t, svc, source.ID).CategoryID)

	count, err := svc.CountCategorySources(category.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestDeleteCategoryReassigningMovesSourcesFirst(t *testing.T) {
	svc := newService(t)

	category := &feed.Category{Name: techCategory}
	require.NoError(t, svc.CreateCategory(category))

	moved := &feed.Source{URL: feedURLA, CategoryID: category.ID}
	kept := &feed.Source{URL: feedURLB, CategoryID: feed.DefaultCategoryID}

	require.NoError(t, svc.CreateSource(moved))
	require.NoError(t, svc.CreateSource(kept))

	count, err := svc.DeleteCategoryReassigning(category.ID, feed.DefaultCategoryID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	_, err = svc.GetCategory(category.ID)
	require.ErrorIs(t, err, service.ErrNotFound)

	// no source is lost and none points at the removed category.
	assert.Equal(t, int64(feed.DefaultCategoryID), reloadSource(t, svc, moved.ID).CategoryID)
	assert.Equal(t, int64(feed.DefaultCategoryID), reloadSource(t, svc, kept.ID).CategoryID)
}

func TestDeleteCategoryReassigningRejectsBadTargets(t *testing.T) {
	svc := newService(t)

	category := &feed.Category{Name: techCategory}
	require.NoError(t, svc.CreateCategory(category))

	source := &feed.Source{URL: feedURLA, CategoryID: category.ID}
	require.NoError(t, svc.CreateSource(source))

	_, err := svc.DeleteCategoryReassigning(category.ID, category.ID)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = svc.DeleteCategoryReassigning(category.ID, 9999)
	require.ErrorIs(t, err, service.ErrNotFound)

	_, err = svc.DeleteCategoryReassigning(feed.DefaultCategoryID, category.ID)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	// every refusal left the data exactly as it was.
	_, err = svc.GetCategory(category.ID)
	require.NoError(t, err)
	assert.Equal(t, category.ID, reloadSource(t, svc, source.ID).CategoryID)
}

func TestListArticlesByCursorPagesWithoutGapsOrDuplicates(t *testing.T) {
	svc := newService(t)

	const total = 25

	for i := range total {
		storeArticle(t, svc, "https://example.com/"+string(rune('a'+i)), nil)
	}

	seen := make([]int64, 0, total)
	cursor := service.ArticleCursor{Limit: 7}

	for range 10 {
		page, err := svc.ListArticlesByCursor(service.ArticleQuery{}, cursor)
		require.NoError(t, err)

		for _, article := range page.Items {
			seen = append(seen, article.ID)
		}

		if !page.HasMore {
			assert.Zero(t, page.NextCursor)

			break
		}

		cursor.Before = page.NextCursor
	}

	require.Len(t, seen, total)

	// strictly descending ids prove there is neither a repeat nor a hole.
	for i := 1; i < len(seen); i++ {
		assert.Greater(t, seen[i-1], seen[i])
	}
}

func TestListArticlesByCursorFiltersBySourceAndCategory(t *testing.T) {
	svc := newService(t)

	category := &feed.Category{Name: techCategory}
	require.NoError(t, svc.CreateCategory(category))

	inCategory := &feed.Source{URL: feedURLA, CategoryID: category.ID}
	elsewhere := &feed.Source{URL: feedURLB, CategoryID: feed.DefaultCategoryID}

	require.NoError(t, svc.CreateSource(inCategory))
	require.NoError(t, svc.CreateSource(elsewhere))

	require.NoError(t, svc.CreateArticle(&feed.Article{URL: "https://example.com/1", SourceID: inCategory.ID}))
	require.NoError(t, svc.CreateArticle(&feed.Article{URL: "https://example.com/2", SourceID: inCategory.ID}))
	require.NoError(t, svc.CreateArticle(&feed.Article{URL: "https://example.com/3", SourceID: elsewhere.ID}))

	bySource, err := svc.ListArticlesByCursor(service.ArticleQuery{SourceID: elsewhere.ID}, service.ArticleCursor{})
	require.NoError(t, err)
	require.Len(t, bySource.Items, 1)
	assert.Equal(t, "https://example.com/3", bySource.Items[0].URL)

	byCategory, err := svc.ListArticlesByCursor(service.ArticleQuery{CategoryID: category.ID}, service.ArticleCursor{})
	require.NoError(t, err)
	assert.Len(t, byCategory.Items, 2)

	// combining both filters intersects them instead of widening the result.
	combined, err := svc.ListArticlesByCursor(
		service.ArticleQuery{CategoryID: category.ID, SourceID: elsewhere.ID}, service.ArticleCursor{})
	require.NoError(t, err)
	assert.Empty(t, combined.Items)
	assert.False(t, combined.HasMore)
}

func TestListArticlesByCursorClampsAndValidates(t *testing.T) {
	svc := newService(t)

	page, err := svc.ListArticlesByCursor(service.ArticleQuery{}, service.ArticleCursor{})
	require.NoError(t, err)
	assert.Equal(t, service.DefaultPageLimit, page.Limit)
	assert.NotNil(t, page.Items)

	page, err = svc.ListArticlesByCursor(service.ArticleQuery{}, service.ArticleCursor{Limit: 10_000})
	require.NoError(t, err)
	assert.Equal(t, service.MaxPageLimit, page.Limit)

	_, err = svc.ListArticlesByCursor(service.ArticleQuery{}, service.ArticleCursor{Before: -1})
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

func categoryNames(t *testing.T, svc *service.Service) []string {
	t.Helper()

	categories, err := svc.AllCategories()
	require.NoError(t, err)

	names := make([]string, 0, len(categories))
	for _, category := range categories {
		names = append(names, category.Name)
	}

	return names
}

func reloadSource(t *testing.T, svc *service.Service, id int64) *feed.Source {
	t.Helper()

	source, err := svc.GetSource(id)
	require.NoError(t, err)

	return source
}
