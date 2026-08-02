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
	"errors"
	"fmt"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// CategoryDTO is one node of the category tree.
type CategoryDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`

	// Sort is the manual integer order; smaller values come first, the id breaks ties.
	Sort int `json:"sort"`

	// SourceCount is how many subscriptions the category currently holds, the number
	// the delete dialog needs before it can offer to move them.
	SourceCount int64 `json:"sourceCount"`

	// IsDefault marks the seeded category, which can be renamed and re-sorted but
	// never deleted, because every orphaned source falls back to it.
	IsDefault bool `json:"isDefault"`
}

// SaveCategoryRequest is the create and update payload of one category.
// A zero ID creates; a non zero ID updates that category.
type SaveCategoryRequest struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Sort int    `json:"sort"`
}

// ListCategories returns the whole category tree in its stable display order.
func (a *App) ListCategories() ([]*CategoryDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	categories, err := a.svc.AllCategories()
	if err != nil {
		return nil, err
	}

	counts, err := a.svc.CategorySourceCounts()
	if err != nil {
		return nil, err
	}

	dtos := make([]*CategoryDTO, 0, len(categories))
	for _, category := range categories {
		dtos = append(dtos, &CategoryDTO{
			ID:          category.ID,
			Name:        category.Name,
			Sort:        category.Sort,
			SourceCount: counts[category.ID],
			IsDefault:   category.ID == feed.DefaultCategoryID,
		})
	}

	return dtos, nil
}

// CreateCategory stores a new category and returns the stored record.
func (a *App) CreateCategory(req *SaveCategoryRequest) (*CategoryDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return nil, err
	}

	category := &feed.Category{Name: req.Name, Sort: req.Sort}

	err = a.svc.CreateCategory(category)
	if err != nil {
		return nil, err
	}

	return &CategoryDTO{ID: category.ID, Name: category.Name, Sort: category.Sort}, nil
}

// UpdateCategory replaces the name and the sort value of one category and returns
// the stored record, the same shape CreateCategory answers with.
//
//nolint:unparam //the frontend contract mirrors CreateCategory even where it reloads instead.
func (a *App) UpdateCategory(req *SaveCategoryRequest) (*CategoryDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return nil, err
	}

	category := &feed.Category{ID: req.ID, Name: req.Name, Sort: req.Sort}

	err = a.svc.UpdateCategory(category)
	if err != nil {
		return nil, err
	}

	count, err := a.svc.CountCategorySources(category.ID)
	if err != nil {
		return nil, err
	}

	return &CategoryDTO{
		ID:          category.ID,
		Name:        category.Name,
		Sort:        category.Sort,
		SourceCount: count,
		IsDefault:   category.ID == feed.DefaultCategoryID,
	}, nil
}

// DeleteCategoryResult is the outcome of one delete attempt.
type DeleteCategoryResult struct {
	// Deleted reports whether the category is gone.
	Deleted bool `json:"deleted"`

	// Moved is the number of subscriptions reassigned before the delete.
	Moved int64 `json:"moved"`

	// InUseCount is the number of subscriptions that blocked the delete. It is only
	// set when Deleted is false, and it is a refusal, not a failure: the frontend
	// asks where the subscriptions should go and calls again.
	InUseCount int64 `json:"inUseCount"`
}

// DeleteCategory removes one category.
//
// The rule is deliberately explicit rather than convenient: with reassignTo left at
// zero the delete is refused while subscriptions still point at the category, and the
// refusal is reported as data so the frontend can offer to move them. Only a second
// call naming the target category actually deletes, and the move plus the delete run
// in one transaction, so a subscription is never silently dropped or left dangling.
func (a *App) DeleteCategory(id, reassignTo int64) (*DeleteCategoryResult, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	if reassignTo != 0 {
		moved, moveErr := a.svc.DeleteCategoryReassigning(id, reassignTo)
		if moveErr != nil {
			return nil, moveErr
		}

		return &DeleteCategoryResult{Deleted: true, Moved: moved}, nil
	}

	err = a.svc.DeleteCategory(id)

	switch {
	case err == nil:
		return &DeleteCategoryResult{Deleted: true}, nil
	case errors.Is(err, service.ErrCategoryInUse):
		count, countErr := a.svc.CountCategorySources(id)
		if countErr != nil {
			return nil, countErr
		}

		return &DeleteCategoryResult{InUseCount: count}, nil
	default:
		return nil, err
	}
}
