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

// DeleteCategoryReassigning removes a category after moving every source it still
// owns to targetID. It is the explicit second half of the delete rule: DeleteCategory
// refuses a category in use, and a caller that wants the delete anyway has to name the
// category the orphans go to. Both statements run in one transaction, so a failure
// leaves neither a dangling source nor a half applied move.
//
// It returns the number of sources actually moved.
func (s *Service) DeleteCategoryReassigning(id, targetID int64) (int64, error) {
	if id == feed.DefaultCategoryID {
		return 0, fmt.Errorf("%w: the default category cannot be deleted", ErrInvalidArgument)
	}

	if targetID == id {
		return 0, fmt.Errorf("%w: cannot reassign sources to the deleted category %d", ErrInvalidArgument, id)
	}

	_, err := s.GetCategory(id)
	if err != nil {
		return 0, err
	}

	err = s.requireCategory(targetID)
	if err != nil {
		return 0, err
	}

	var moved int64

	err = s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&feed.Source{}).Where("category_id = ?", id).Update("category_id", targetID)
		if result.Error != nil {
			return fmt.Errorf("move sources of category %d to %d: %w", id, targetID, result.Error)
		}

		moved = result.RowsAffected

		deleteErr := tx.Where("id = ?", id).Delete(&feed.Category{}).Error
		if deleteErr != nil {
			return fmt.Errorf("delete category %d: %w", id, deleteErr)
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return moved, nil
}

// CountCategorySources reports how many sources point at one category, the number the
// UI shows before asking whether the sources should be moved or the delete canceled.
func (s *Service) CountCategorySources(id int64) (int64, error) {
	var count int64

	err := s.db.Model(&feed.Source{}).Where("category_id = ?", id).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count sources of category %d: %w", id, err)
	}

	return count, nil
}

// CategorySourceCounts returns the number of sources of every category in one query,
// so the category tree can show its counts without a lookup per row.
func (s *Service) CategorySourceCounts() (map[int64]int64, error) {
	var rows []struct {
		CategoryID int64
		Total      int64
	}

	err := s.db.Model(&feed.Source{}).
		Select("category_id, count(*) as total").
		Group("category_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count sources per category: %w", err)
	}

	counts := make(map[int64]int64, len(rows))
	for _, row := range rows {
		counts[row.CategoryID] = row.Total
	}

	return counts, nil
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
