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

// SourceQuery filters a source listing.
type SourceQuery struct {
	// CategoryID restricts the result to one category when it is greater than zero.
	CategoryID int64 `json:"category_id"`

	// Enabled restricts the result to enabled or disabled sources when it is not nil.
	Enabled *bool `json:"enabled"`

	// Keyword matches the title or the url when it is not empty.
	Keyword string `json:"keyword"`
}

// CreateSource stores a new source. A source is enabled by default and belongs to
// the default category unless the caller says otherwise.
func (s *Service) CreateSource(source *feed.Source) error {
	if source == nil {
		return fmt.Errorf("%w: source is nil", ErrInvalidArgument)
	}

	if source.URL == "" {
		return fmt.Errorf("%w: source url is empty", ErrInvalidArgument)
	}

	if source.ParseType != "" && !feed.IsLegalParseType(source.ParseType) {
		return fmt.Errorf("%w: unknown parse type %q", ErrInvalidArgument, source.ParseType)
	}

	if source.CategoryID == 0 {
		source.CategoryID = feed.DefaultCategoryID
	}

	if err := s.requireCategory(source.CategoryID); err != nil {
		return err
	}

	// a newly created source is enabled; disabling is an explicit follow up update.
	source.Enabled = true

	if err := s.db.Create(source).Error; err != nil {
		return fmt.Errorf("create source: %w", err)
	}

	return nil
}

// GetSource reads one source by id.
func (s *Service) GetSource(id int64) (*feed.Source, error) {
	var source feed.Source

	if err := s.db.Where("id = ?", id).First(&source).Error; err != nil {
		return nil, wrapFind(err, "source", id)
	}

	return &source, nil
}

// UpdateSource replaces every column of an existing source.
func (s *Service) UpdateSource(source *feed.Source) error {
	if source == nil || source.ID == 0 {
		return fmt.Errorf("%w: source id is required", ErrInvalidArgument)
	}

	if source.ParseType != "" && !feed.IsLegalParseType(source.ParseType) {
		return fmt.Errorf("%w: unknown parse type %q", ErrInvalidArgument, source.ParseType)
	}

	if source.CategoryID == 0 {
		source.CategoryID = feed.DefaultCategoryID
	}

	if err := s.requireCategory(source.CategoryID); err != nil {
		return err
	}

	if _, err := s.GetSource(source.ID); err != nil {
		return err
	}

	if err := s.db.Save(source).Error; err != nil {
		return fmt.Errorf("update source %d: %w", source.ID, err)
	}

	return nil
}

// UpdateSourceColumn updates a single column, the shape the CLI update command uses.
func (s *Service) UpdateSourceColumn(id int64, column string, value any) error {
	if column == "" {
		return fmt.Errorf("%w: column is empty", ErrInvalidArgument)
	}

	if _, err := s.GetSource(id); err != nil {
		return err
	}

	if err := s.db.Model(&feed.Source{}).Where("id = ?", id).Update(column, value).Error; err != nil {
		return fmt.Errorf("update source %d column %q: %w", id, column, err)
	}

	return nil
}

// SetSourceEnabled turns a source on or off for real fetches.
func (s *Service) SetSourceEnabled(id int64, enabled bool) error {
	return s.UpdateSourceColumn(id, "enabled", enabled)
}

// DeleteSource removes a source by id.
func (s *Service) DeleteSource(id int64) error {
	result := s.db.Where("id = ?", id).Delete(&feed.Source{})
	if result.Error != nil {
		return fmt.Errorf("delete source %d: %w", id, result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: source %d", ErrNotFound, id)
	}

	return nil
}

// ListSources returns one page of sources ordered by id.
func (s *Service) ListSources(query SourceQuery, page PageRequest) (*Page[*feed.Source], error) {
	return findPage[*feed.Source](s.sourceQuery(query), "id asc", page)
}

// AllSources returns every source ordered by id, for callers that need the whole set.
func (s *Service) AllSources(query SourceQuery) ([]*feed.Source, error) {
	var sources []*feed.Source

	if err := s.sourceQuery(query).Order("id asc").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}

	return sources, nil
}

func (s *Service) sourceQuery(query SourceQuery) *gorm.DB {
	db := s.db.Model(&feed.Source{})

	if query.CategoryID > 0 {
		db = db.Where("category_id = ?", query.CategoryID)
	}

	if query.Enabled != nil {
		db = db.Where("enabled = ?", *query.Enabled)
	}

	if query.Keyword != "" {
		like := "%" + query.Keyword + "%"
		db = db.Where("title LIKE ? OR url LIKE ?", like, like)
	}

	return db
}
