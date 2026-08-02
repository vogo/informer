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
	"fmt"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// ArticleItemDTO is one row of the article library.
type ArticleItemDTO struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`

	SourceID int64 `json:"sourceId"`

	// SourceTitle is resolved here so the list does not need a lookup per row.
	// It is empty when the source that produced the article no longer exists.
	SourceTitle string `json:"sourceTitle"`

	// CategoryID is the category of the producing source, zero when it is gone.
	CategoryID int64 `json:"categoryId"`

	Informed bool `json:"informed"`

	// InformedAt is the unix second of the delivery, zero when it is unknown.
	// A historical article keeps a zero here until the history index is rebuilt.
	InformedAt int64 `json:"informedAt"`

	// FetchedAt is the unix second the article was first stored, zero when unknown.
	FetchedAt int64 `json:"fetchedAt"`

	Score int64 `json:"score"`
}

// ArticleQueryRequest filters the article library. A zero id disables that filter.
type ArticleQueryRequest struct {
	SourceID   int64  `json:"sourceId"`
	CategoryID int64  `json:"categoryId"`
	Keyword    string `json:"keyword"`

	// Before is the cursor: only articles with a smaller id are returned.
	// Zero starts at the newest article, which is what a filter change resets to.
	Before int64 `json:"before"`

	Limit int `json:"limit"`
}

// ArticlePageDTO is one cursor page of the article library.
type ArticlePageDTO struct {
	Items []*ArticleItemDTO `json:"items"`

	// NextCursor is the Before value of the following page, zero on the last page.
	NextCursor int64 `json:"nextCursor"`

	HasMore bool `json:"hasMore"`
	Limit   int  `json:"limit"`
}

// ListArticles returns one cursor page of the article library, newest first.
//
// The cursor is the article id, so a page boundary stays correct while a scheduled
// run inserts new articles underneath - an offset would shift and start repeating
// rows. Paging back is the frontend's job: it keeps the cursor of every page it
// opened, because the previous cursor cannot be derived from the current page.
func (a *App) ListArticles(req *ArticleQueryRequest) (*ArticlePageDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return nil, err
	}

	page, err := a.svc.ListArticlesByCursor(
		service.ArticleQuery{SourceID: req.SourceID, CategoryID: req.CategoryID, Keyword: req.Keyword},
		service.ArticleCursor{Before: req.Before, Limit: req.Limit},
	)
	if err != nil {
		return nil, err
	}

	sources, err := a.svc.AllSources(service.SourceQuery{})
	if err != nil {
		return nil, err
	}

	byID := make(map[int64]*feed.Source, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}

	items := make([]*ArticleItemDTO, 0, len(page.Items))
	for _, article := range page.Items {
		items = append(items, toArticleItemDTO(article, byID[article.SourceID]))
	}

	return &ArticlePageDTO{
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
		Limit:      page.Limit,
	}, nil
}

// toArticleItemDTO flattens one article and the source that produced it.
func toArticleItemDTO(article *feed.Article, source *feed.Source) *ArticleItemDTO {
	dto := &ArticleItemDTO{
		ID:       article.ID,
		Title:    article.Title,
		URL:      article.URL,
		SourceID: article.SourceID,
		Informed: article.Informed,
		Score:    article.Score,
	}

	// a nil timestamp is "not recorded"; it is flattened to zero rather than to a
	// fake date, so the library can say so instead of inventing a day.
	if article.InformedAt != nil {
		dto.InformedAt = *article.InformedAt
	}

	if article.FetchedAt != nil {
		dto.FetchedAt = *article.FetchedAt
	}

	if source != nil {
		dto.SourceTitle = source.Title
		if dto.SourceTitle == "" {
			dto.SourceTitle = source.URL
		}

		dto.CategoryID = source.CategoryID
	}

	return dto
}
