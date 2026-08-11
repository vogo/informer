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
	"strings"

	"gorm.io/gorm"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
)

// Fetch health states a source listing can be restricted to. They name the same
// three buckets the subscription cards show, so a filter can never select a
// state the card labels differently.
const (
	// FetchStatusNormal is feed.StatusNormal: the last fetch succeeded.
	FetchStatusNormal = "normal"

	// FetchStatusError is feed.StatusError: the last fetch failed.
	FetchStatusError = "error"

	// FetchStatusUnfetched is every other stored value, zero included: the
	// source has no recorded fetch outcome yet.
	FetchStatusUnfetched = "unfetched"
)

// SourceQuery filters a source listing. Every field is optional and the set of
// filled ones is combined with AND; an empty query lists everything.
type SourceQuery struct {
	// CategoryID restricts the result to one category when it is greater than zero.
	CategoryID int64 `json:"category_id"`

	// Enabled restricts the result to enabled or disabled sources when it is not nil.
	Enabled *bool `json:"enabled"`

	// Keyword matches the title or the url when it is not empty.
	Keyword string `json:"keyword"`

	// ParseType restricts the result to one resolved parse type when it is not
	// empty. It matches feed.Source.ResolveParseType, not the stored column, so
	// a record that still derives its type from the historical IsJSON and Regex
	// fields is filtered exactly as the list renders it. An unsupported value is
	// an ErrInvalidArgument rather than a silent "everything".
	ParseType string `json:"parse_type"`

	// FetchStatus restricts the result to one of FetchStatusNormal,
	// FetchStatusError or FetchStatusUnfetched when it is not empty. Any other
	// value is an ErrInvalidArgument.
	FetchStatus string `json:"fetch_status"`
}

// ValidateSource refuses a source its resolved parser could not act on.
//
// What "unusable" means depends on the parse type: a fetching source needs an
// address, while an agent source needs an instruction and no address at all. Both
// rules live here so that creating, updating and previewing a source agree on what
// a usable source is.
//
// The stored parse type is resolved, not judged: a value written by a build this
// one does not know about keeps falling back to the historical derivation, exactly
// as a fetch does. Refusing an unknown name is the job of the two write paths.
func ValidateSource(source *feed.Source) error {
	if source == nil {
		return fmt.Errorf("%w: source is nil", ErrInvalidArgument)
	}

	if source.ResolveParseType() != feed.ParseTypeAgent {
		if source.URL == "" && source.CURL == "" {
			return fmt.Errorf("%w: source has neither url nor curl", ErrInvalidArgument)
		}

		return nil
	}

	if strings.TrimSpace(source.AgentPrompt) == "" {
		return fmt.Errorf("%w: agent source has no prompt", ErrInvalidArgument)
	}

	provider := strings.TrimSpace(source.AgentProvider)
	if provider != "" && !agent.IsLegalProvider(provider) {
		return fmt.Errorf("%w: unknown agent provider %q", ErrInvalidArgument, provider)
	}

	return nil
}

// CreateSource stores a new source. A source is enabled by default and belongs to
// the default category unless the caller says otherwise.
func (s *Service) CreateSource(source *feed.Source) error {
	if source == nil {
		return fmt.Errorf("%w: source is nil", ErrInvalidArgument)
	}

	if source.ParseType != "" && !feed.IsLegalParseType(source.ParseType) {
		return fmt.Errorf("%w: unknown parse type %q", ErrInvalidArgument, source.ParseType)
	}

	err := ValidateSource(source)
	if err != nil {
		return err
	}

	// a fetching source is created from its url alone, the shape every existing
	// caller uses; only an agent source is allowed to carry no address.
	if source.URL == "" && source.ResolveParseType() != feed.ParseTypeAgent {
		return fmt.Errorf("%w: source url is empty", ErrInvalidArgument)
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

	err := ValidateSource(source)
	if err != nil {
		return err
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
	db, err := s.sourceQuery(query)
	if err != nil {
		return nil, err
	}

	return findPage[*feed.Source](db, "id asc", page)
}

// AllSources returns every source ordered by id, for callers that need the whole set.
func (s *Service) AllSources(query SourceQuery) ([]*feed.Source, error) {
	db, err := s.sourceQuery(query)
	if err != nil {
		return nil, err
	}

	var sources []*feed.Source

	err = db.Order("id asc").Find(&sources).Error
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}

	return sources, nil
}

func (s *Service) sourceQuery(query SourceQuery) (*gorm.DB, error) {
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

	if query.ParseType != "" {
		condition, err := resolvedParseTypeCondition(query.ParseType)
		if err != nil {
			return nil, err
		}

		db = db.Where(condition.sql, condition.args...)
	}

	if query.FetchStatus != "" {
		condition, err := fetchStatusCondition(query.FetchStatus)
		if err != nil {
			return nil, err
		}

		db = db.Where(condition.sql, condition.args...)
	}

	return db, nil
}

// sourceCondition is one SQL fragment and the arguments it binds.
type sourceCondition struct {
	sql  string
	args []any
}

// resolvedParseTypeCondition renders feed.Source.ResolveParseType as SQL: the
// explicit column wins when it holds a supported value, otherwise the historical
// derivation from IsJSON and Regex decides. Both branches are needed because a
// record written before the parse type column existed still carries an empty -
// or an unknown, since removed - value there.
func resolvedParseTypeCondition(parseType string) (sourceCondition, error) {
	if !feed.IsLegalParseType(parseType) {
		return sourceCondition{}, fmt.Errorf("%w: unknown parse type %q", ErrInvalidArgument, parseType)
	}

	// the explicit branch alone already answers the whole question for a parse
	// type the legacy columns cannot express - a parser added after they were
	// frozen - because deriving would never name it either.
	const explicit = "COALESCE(parse_type, '') = ?"

	legacy := legacyParseTypeCondition(parseType)
	if legacy.sql == "" {
		return sourceCondition{sql: "(" + explicit + ")", args: []any{parseType}}, nil
	}

	return sourceCondition{
		sql:  "(" + explicit + " OR (COALESCE(parse_type, '') NOT IN ? AND " + legacy.sql + "))",
		args: append([]any{parseType, feed.LegalParseTypes()}, legacy.args...),
	}, nil
}

// legacyParseTypeCondition mirrors feed.DeriveParseType over the historical
// columns. It returns an empty condition for a parse type the old fields have
// no way to express.
func legacyParseTypeCondition(parseType string) sourceCondition {
	switch parseType {
	case feed.ParseTypeJSON:
		return sourceCondition{sql: "is_json = ?", args: []any{true}}
	case feed.ParseTypeRegex:
		return sourceCondition{sql: "is_json = ? AND COALESCE(regex, '') != ''", args: []any{false}}
	case feed.ParseTypeFeed:
		return sourceCondition{sql: "is_json = ? AND COALESCE(regex, '') = ''", args: []any{false}}
	default:
		return sourceCondition{}
	}
}

// fetchStatusCondition maps one health bucket to SQL. "Unfetched" is defined as
// the complement of the two known states, so a stray value stored by an older
// build lands in the same bucket the card shows it in instead of disappearing
// from every filter.
func fetchStatusCondition(status string) (sourceCondition, error) {
	const knownStatus = "status = ?"

	switch status {
	case FetchStatusNormal:
		return sourceCondition{sql: knownStatus, args: []any{feed.StatusNormal}}, nil
	case FetchStatusError:
		return sourceCondition{sql: knownStatus, args: []any{feed.StatusError}}, nil
	case FetchStatusUnfetched:
		return sourceCondition{
			sql:  "COALESCE(status, 0) NOT IN ?",
			args: []any{[]int{feed.StatusNormal, feed.StatusError}},
		}, nil
	default:
		return sourceCondition{}, fmt.Errorf("%w: unknown fetch status %q", ErrInvalidArgument, status)
	}
}
