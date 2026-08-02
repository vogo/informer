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

	"gorm.io/gorm"

	"github.com/vogo/informer/internal/feed"
)

// articleOrder is the stable listing order of articles: newest id first.
const articleOrder = "articles.id desc"

// ArticleQuery filters an article listing. Source and category filters are resolved
// in the database so that no caller has to assemble sql of its own.
type ArticleQuery struct {
	// SourceID restricts the result to one source when it is greater than zero.
	SourceID int64 `json:"source_id"`

	// CategoryID restricts the result to the sources of one category when greater than zero.
	CategoryID int64 `json:"category_id"`

	// Informed restricts the result to informed or pending articles when it is not nil.
	Informed *bool `json:"informed"`

	// Keyword matches the title or the url when it is not empty.
	Keyword string `json:"keyword"`
}

// CreateArticle stores a new article.
func (s *Service) CreateArticle(article *feed.Article) error {
	if article == nil {
		return fmt.Errorf("%w: article is nil", ErrInvalidArgument)
	}

	if article.URL == "" {
		return fmt.Errorf("%w: article url is empty", ErrInvalidArgument)
	}

	if err := s.db.Create(article).Error; err != nil {
		return fmt.Errorf("create article: %w", err)
	}

	return nil
}

// GetArticle reads one article by id.
func (s *Service) GetArticle(id int64) (*feed.Article, error) {
	var article feed.Article

	if err := s.db.Where("id = ?", id).First(&article).Error; err != nil {
		return nil, wrapFind(err, "article", id)
	}

	return &article, nil
}

// UpdateArticle replaces every column of an existing article.
func (s *Service) UpdateArticle(article *feed.Article) error {
	if article == nil || article.ID == 0 {
		return fmt.Errorf("%w: article id is required", ErrInvalidArgument)
	}

	if _, err := s.GetArticle(article.ID); err != nil {
		return err
	}

	if err := s.db.Save(article).Error; err != nil {
		return fmt.Errorf("update article %d: %w", article.ID, err)
	}

	return nil
}

// DeleteArticle removes an article by id.
func (s *Service) DeleteArticle(id int64) error {
	result := s.db.Where("id = ?", id).Delete(&feed.Article{})
	if result.Error != nil {
		return fmt.Errorf("delete article %d: %w", id, result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: article %d", ErrNotFound, id)
	}

	return nil
}

// ArticleCursor asks for one slice of the descending id ordered article list.
type ArticleCursor struct {
	// Before is exclusive: only articles with a smaller id are returned.
	// A zero value starts at the newest article.
	Before int64 `json:"before"`

	// Limit is the page size, clamped into the supported bounds.
	Limit int `json:"limit"`
}

// ArticleCursorPage is one cursor page of articles, newest first.
type ArticleCursorPage struct {
	Items []*feed.Article `json:"items"`

	// NextCursor is the Before value of the following page. It is zero when the
	// page is the last one, so a caller can stop without a second query.
	NextCursor int64 `json:"next_cursor"`

	// Limit is the page size actually applied after clamping.
	Limit int `json:"limit"`

	HasMore bool `json:"has_more"`
}

// ListArticles returns one page of articles, newest first.
func (s *Service) ListArticles(query ArticleQuery, page PageRequest) (*Page[*feed.Article], error) {
	return findPage[*feed.Article](s.articleQuery(query), articleOrder, page)
}

// ListArticlesByCursor returns one cursor page of articles, newest id first.
//
// The cursor is the article id itself, which is unique and never reused, so a page
// boundary cannot drop or repeat a row while articles are being inserted - unlike an
// offset, which shifts under every new article. Paging back is the caller's job: it
// remembers the cursor of each page it opened, because a stable "previous" cursor
// cannot be derived from the current page alone.
func (s *Service) ListArticlesByCursor(query ArticleQuery, cursor ArticleCursor) (*ArticleCursorPage, error) {
	limit := cursor.Limit
	if limit <= 0 {
		limit = DefaultPageLimit
	}

	if limit > MaxPageLimit {
		limit = MaxPageLimit
	}

	if cursor.Before < 0 {
		return nil, fmt.Errorf("%w: article cursor %d is negative", ErrInvalidArgument, cursor.Before)
	}

	db := s.articleQuery(query)
	if cursor.Before > 0 {
		db = db.Where("articles.id < ?", cursor.Before)
	}

	// one extra row answers "is there a next page" without a second count query.
	var items []*feed.Article

	err := db.Order(articleOrder).Limit(limit + 1).Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list articles by cursor: %w", err)
	}

	page := &ArticleCursorPage{Items: items, Limit: limit}

	if len(items) > limit {
		page.Items = items[:limit]
		page.HasMore = true
		page.NextCursor = page.Items[limit-1].ID
	}

	if page.Items == nil {
		page.Items = []*feed.Article{}
	}

	return page, nil
}

func (s *Service) articleQuery(query ArticleQuery) *gorm.DB {
	db := s.db.Model(&feed.Article{})

	if query.SourceID > 0 {
		db = db.Where("articles.source_id = ?", query.SourceID)
	}

	if query.CategoryID > 0 {
		db = db.Where("articles.source_id IN (?)",
			s.db.Model(&feed.Source{}).Select("id").Where("category_id = ?", query.CategoryID))
	}

	if query.Informed != nil {
		db = db.Where("articles.informed = ?", *query.Informed)
	}

	if query.Keyword != "" {
		like := "%" + query.Keyword + "%"
		db = db.Where("articles.title LIKE ? OR articles.url LIKE ?", like, like)
	}

	return db
}
