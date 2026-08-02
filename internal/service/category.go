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
)

// categoryOrder is the stable listing order: the manual sort value first, id as tie breaker.
const categoryOrder = "sort asc, id asc"

// CreateCategory stores a new category.
func (s *Service) CreateCategory(category *feed.Category) error {
	if category == nil || category.Name == "" {
		return fmt.Errorf("%w: category name is empty", ErrInvalidArgument)
	}

	if err := s.db.Create(category).Error; err != nil {
		return fmt.Errorf("create category: %w", err)
	}

	return nil
}

// GetCategory reads one category by id.
func (s *Service) GetCategory(id int64) (*feed.Category, error) {
	var category feed.Category

	if err := s.db.Where("id = ?", id).First(&category).Error; err != nil {
		return nil, wrapFind(err, "category", id)
	}

	return &category, nil
}

// UpdateCategory replaces the name and the sort value of an existing category.
func (s *Service) UpdateCategory(category *feed.Category) error {
	if category == nil || category.ID == 0 {
		return fmt.Errorf("%w: category id is required", ErrInvalidArgument)
	}

	if category.Name == "" {
		return fmt.Errorf("%w: category name is empty", ErrInvalidArgument)
	}

	if _, err := s.GetCategory(category.ID); err != nil {
		return err
	}

	if err := s.db.Save(category).Error; err != nil {
		return fmt.Errorf("update category %d: %w", category.ID, err)
	}

	return nil
}

// DeleteCategory removes a category. The default category is never removed, and a
// category still referenced by sources is refused so that no source is left dangling.
func (s *Service) DeleteCategory(id int64) error {
	if id == feed.DefaultCategoryID {
		return fmt.Errorf("%w: the default category cannot be deleted", ErrInvalidArgument)
	}

	if _, err := s.GetCategory(id); err != nil {
		return err
	}

	var referencing int64
	if err := s.db.Model(&feed.Source{}).Where("category_id = ?", id).Count(&referencing).Error; err != nil {
		return fmt.Errorf("count sources of category %d: %w", id, err)
	}

	if referencing > 0 {
		return fmt.Errorf("%w: category %d has %d sources", ErrCategoryInUse, id, referencing)
	}

	if err := s.db.Where("id = ?", id).Delete(&feed.Category{}).Error; err != nil {
		return fmt.Errorf("delete category %d: %w", id, err)
	}

	return nil
}

// ListCategories returns one page of categories ordered by sort value, then id.
func (s *Service) ListCategories(page PageRequest) (*Page[*feed.Category], error) {
	return findPage[*feed.Category](s.db.Model(&feed.Category{}), categoryOrder, page)
}

// AllCategories returns every category in the stable listing order.
func (s *Service) AllCategories() ([]*feed.Category, error) {
	var categories []*feed.Category

	if err := s.db.Model(&feed.Category{}).Order(categoryOrder).Find(&categories).Error; err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}

	return categories, nil
}

// requireCategory fails when the referenced category does not exist.
func (s *Service) requireCategory(id int64) error {
	if _, err := s.GetCategory(id); err != nil {
		return err
	}

	return nil
}
