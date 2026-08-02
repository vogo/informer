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

	"github.com/vogo/informer/internal/feed"
	"gorm.io/gorm"
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

// ListArticles returns one page of articles, newest first.
func (s *Service) ListArticles(query ArticleQuery, page PageRequest) (*Page[*feed.Article], error) {
	return findPage[*feed.Article](s.articleQuery(query), articleOrder, page)
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
