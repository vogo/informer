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

// Package service holds the business entry point of informer. It owns data access
// and orchestration so that every front end - the CLI today, a desktop UI later -
// talks to the same layer instead of driving the database and the feed package directly.
//
// The service returns domain results and errors: it never prints, never exits the
// process and carries no presentation specific types.
package service

import (
	"errors"
	"fmt"

	"github.com/vogo/informer/internal/feed"
	"gorm.io/gorm"
)

// Errors returned by the service layer.
var (
	// ErrNotFound marks a record that does not exist.
	ErrNotFound = errors.New("record not found")

	// ErrInvalidArgument marks a caller supplied value the service refuses.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrCategoryInUse marks a category that still has sources pointing at it.
	ErrCategoryInUse = errors.New("category is still referenced by sources")
)

// Default paging bounds. A request without an explicit limit gets DefaultPageLimit.
const (
	DefaultPageLimit = 20
	MaxPageLimit     = 500
)

// Service is the business entry point backed by the feed database of one data directory.
type Service struct {
	db      *gorm.DB
	homeDir string
}

// New opens the feed database inside homeDir and returns the service bound to it.
func New(homeDir string) (*Service, error) {
	if homeDir == "" {
		return nil, fmt.Errorf("%w: home dir is empty", ErrInvalidArgument)
	}

	db, err := feed.InitFeedDB(homeDir)
	if err != nil {
		return nil, err
	}

	return &Service{db: db, homeDir: homeDir}, nil
}

// HomeDir returns the active data directory the service reads and writes.
func (s *Service) HomeDir() string {
	return s.homeDir
}

// DB exposes the underlying handle for tests and for the feed package internals.
func (s *Service) DB() *gorm.DB {
	return s.db
}

// PageRequest asks for one slice of an ordered result set.
type PageRequest struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// normalized clamps the request into the supported bounds.
func (p PageRequest) normalized() PageRequest {
	offset := p.Offset
	if offset < 0 {
		offset = 0
	}

	limit := p.Limit
	if limit <= 0 {
		limit = DefaultPageLimit
	}

	if limit > MaxPageLimit {
		limit = MaxPageLimit
	}

	return PageRequest{Offset: offset, Limit: limit}
}

// Page is one page of a stably ordered result set, carrying everything a caller
// needs to continue querying.
type Page[T any] struct {
	Items   []T   `json:"items"`
	Total   int64 `json:"total"`
	Offset  int   `json:"offset"`
	Limit   int   `json:"limit"`
	HasMore bool  `json:"has_more"`
}

// newPage builds a page result from a normalized request.
func newPage[T any](items []T, total int64, req PageRequest) *Page[T] {
	if items == nil {
		items = []T{}
	}

	return &Page[T]{
		Items:   items,
		Total:   total,
		Offset:  req.Offset,
		Limit:   req.Limit,
		HasMore: int64(req.Offset+len(items)) < total,
	}
}

// findPage runs the count and the ordered slice query of one paged lookup.
func findPage[T any](query *gorm.DB, order string, req PageRequest) (*Page[T], error) {
	req = req.normalized()

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count records: %w", err)
	}

	var items []T
	if err := query.Order(order).Offset(req.Offset).Limit(req.Limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}

	return newPage(items, total, req), nil
}

// wrapFind turns a gorm lookup error into a service error.
func wrapFind(err error, what string, id int64) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %s %d", ErrNotFound, what, id)
	}

	return fmt.Errorf("read %s %d: %w", what, id, err)
}
