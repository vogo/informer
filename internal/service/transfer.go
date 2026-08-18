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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/vogo/informer/internal/feed"
)

// SourceExportVersion is the schema version written into every export document.
// A file claiming a newer version is refused rather than imported with the
// fields this build happens to know, so an old build never silently drops what
// a newer one wrote.
const SourceExportVersion = 1

// SourceExport is one portable subscription: every configuration column a source
// owns, and nothing that belongs to one machine. The database id is left out
// because it means nothing on another installation, the fetch health (status and
// error info) because it describes this machine's last fetch, and the category is
// carried by name because category ids differ per installation.
type SourceExport struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	CURL          string `json:"curl,omitempty"`
	Weight        int64  `json:"weight,omitempty"`
	MaxFetchNum   int    `json:"max_fetch_num,omitempty"`
	Regex         string `json:"regex,omitempty"`
	TitleExp      string `json:"title_exp,omitempty"`
	URLExp        string `json:"url_exp,omitempty"`
	Redirect      bool   `json:"redirect,omitempty"`
	Sort          bool   `json:"sort,omitempty"`
	IsJSON        bool   `json:"is_json,omitempty"`
	JSONTitlePath string `json:"json_title_path,omitempty"`
	JSONURLPath   string `json:"json_url_path,omitempty"`
	ParseType     string `json:"parse_type,omitempty"`
	AgentProvider string `json:"agent_provider,omitempty"`
	AgentPrompt   string `json:"agent_prompt,omitempty"`

	// Category is the category name; an empty name lands in the default category.
	Category string `json:"category,omitempty"`

	// Enabled is written out even when false: a disabled subscription has to
	// come back disabled, so this field is never omitted.
	Enabled bool `json:"enabled"`
}

// SourceExportDoc is the whole export file.
type SourceExportDoc struct {
	Version int `json:"version"`

	// ExportedAt is informational only; the import never reads it.
	ExportedAt string `json:"exported_at,omitempty"`

	Sources []*SourceExport `json:"sources"`
}

// ImportSourcesResult reports what one import did, entry by entry. A single
// unusable entry never aborts the run: it is counted in Failed and described in
// Errors, and every other entry is still applied.
type ImportSourcesResult struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Failed  int `json:"failed"`

	// Errors describes every failed entry, one message per entry.
	Errors []string `json:"errors"`
}

// ExportSources returns every stored subscription as a portable document,
// ordered by id like every other listing.
func (s *Service) ExportSources() (*SourceExportDoc, error) {
	sources, err := s.AllSources(SourceQuery{})
	if err != nil {
		return nil, err
	}

	categories, err := s.AllCategories()
	if err != nil {
		return nil, err
	}

	names := make(map[int64]string, len(categories))
	for _, category := range categories {
		names[category.ID] = category.Name
	}

	doc := &SourceExportDoc{
		Version:    SourceExportVersion,
		ExportedAt: time.Now().Format(time.RFC3339),
		Sources:    make([]*SourceExport, 0, len(sources)),
	}

	for _, source := range sources {
		doc.Sources = append(doc.Sources, toSourceExport(source, names[source.CategoryID]))
	}

	return doc, nil
}

// MarshalSourceExport renders one export document as the indented json written
// to disk, so every writer of an export file produces the same layout.
func MarshalSourceExport(doc *SourceExportDoc) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode source export: %w", err)
	}

	return append(data, '\n'), nil
}

// ParseSourceExport reads an export document. A bare json array of subscriptions
// is accepted as well, so a hand written list needs no wrapper object.
func ParseSourceExport(data []byte) (*SourceExportDoc, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: the import file is empty", ErrInvalidArgument)
	}

	if trimmed[0] == '[' {
		var sources []*SourceExport

		err := json.Unmarshal(trimmed, &sources)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
		}

		return &SourceExportDoc{Sources: sources}, nil
	}

	var doc SourceExportDoc

	err := json.Unmarshal(trimmed, &doc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}

	return &doc, nil
}

// ImportSourcesJSON parses one export file and applies it.
func (s *Service) ImportSourcesJSON(data []byte) (*ImportSourcesResult, error) {
	doc, err := ParseSourceExport(data)
	if err != nil {
		return nil, err
	}

	return s.ImportSources(doc)
}

// ImportSources merges an export document into the stored subscriptions: an entry
// matching a stored subscription overwrites its configuration, an entry matching
// none is appended. Nothing is ever removed, so importing a partial file only
// adds to and updates what is already there.
//
// The identity of a subscription across installations is its url; a subscription
// without one - an agent subscription has no address at all - is identified by
// its title. Matching by title also means renaming an agent subscription and
// importing the old file creates a second one, which is the only honest answer
// when the file carries nothing else to recognize it by.
//
// The fetch health of a matched subscription (status and error info) is kept: it
// describes this machine's last fetch, not the imported configuration.
func (s *Service) ImportSources(doc *SourceExportDoc) (*ImportSourcesResult, error) {
	if doc == nil {
		return nil, fmt.Errorf("%w: import document is nil", ErrInvalidArgument)
	}

	if doc.Version > SourceExportVersion {
		return nil, fmt.Errorf("%w: export version %d is newer than the supported version %d",
			ErrInvalidArgument, doc.Version, SourceExportVersion)
	}

	index, err := s.storedSourceIndex()
	if err != nil {
		return nil, err
	}

	categories, err := s.AllCategories()
	if err != nil {
		return nil, err
	}

	categoryIDs := make(map[string]int64, len(categories))
	for _, category := range categories {
		categoryIDs[category.Name] = category.ID
	}

	result := &ImportSourcesResult{Total: len(doc.Sources)}

	for i, entry := range doc.Sources {
		created, importErr := s.importSource(entry, index, categoryIDs)
		if importErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("entry %d %s: %v", i+1, entryLabel(entry), importErr))

			continue
		}

		if created {
			result.Created++
		} else {
			result.Updated++
		}
	}

	return result, nil
}

// storedSourceIndex maps the identity of every stored subscription to it. The
// first record of a duplicated key wins, so a database that already holds the
// same url twice keeps updating the same one instead of alternating.
func (s *Service) storedSourceIndex() (map[string]*feed.Source, error) {
	stored, err := s.AllSources(SourceQuery{})
	if err != nil {
		return nil, err
	}

	index := make(map[string]*feed.Source, len(stored))

	for _, source := range stored {
		key := sourceKey(source.URL, source.Title)
		if key == "" {
			continue
		}

		if _, exists := index[key]; !exists {
			index[key] = source
		}
	}

	return index, nil
}

// importSource applies one entry and reports whether it created a subscription.
// The index is updated with what it wrote, so a file listing the same
// subscription twice updates it the second time instead of duplicating it.
func (s *Service) importSource(
	entry *SourceExport,
	index map[string]*feed.Source,
	categoryIDs map[string]int64,
) (bool, error) {
	if entry == nil {
		return false, fmt.Errorf("%w: entry is nil", ErrInvalidArgument)
	}

	key := sourceKey(entry.URL, entry.Title)
	if key == "" {
		return false, fmt.Errorf("%w: entry has neither url nor title", ErrInvalidArgument)
	}

	categoryID, err := s.importCategoryID(entry.Category, categoryIDs)
	if err != nil {
		return false, err
	}

	stored, found := index[key]
	if found {
		applySourceExport(stored, entry, categoryID)

		err = s.UpdateSource(stored)
		if err != nil {
			return false, err
		}

		return false, nil
	}

	source := applySourceExport(&feed.Source{}, entry, categoryID)

	err = s.CreateSource(source)
	if err != nil {
		return false, err
	}

	// a created source is enabled by rule; a disabled entry has to be turned
	// off right after, so an import restores exactly the state it carries.
	if !entry.Enabled {
		err = s.SetSourceEnabled(source.ID, false)
		if err != nil {
			return false, err
		}

		source.Enabled = false
	}

	index[key] = source

	return true, nil
}

// importCategoryID resolves the category name of one entry, creating the
// category when this installation does not have it yet. An empty name is the
// default category, the same fallback a source saved without one gets.
func (s *Service) importCategoryID(name string, categoryIDs map[string]int64) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return feed.DefaultCategoryID, nil
	}

	if id, found := categoryIDs[name]; found {
		return id, nil
	}

	category := &feed.Category{Name: name}

	err := s.CreateCategory(category)
	if err != nil {
		return 0, err
	}

	categoryIDs[name] = category.ID

	return category.ID, nil
}

// sourceKey is the cross installation identity of one subscription: its url, or
// its title when it has no url. The two are kept in separate namespaces so a
// title that happens to read like an address cannot match a url. An entry with
// neither returns an empty key and is refused by the caller.
func sourceKey(url, title string) string {
	trimmedURL := strings.TrimSpace(url)
	if trimmedURL != "" {
		return "url:" + trimmedURL
	}

	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle != "" {
		return "title:" + trimmedTitle
	}

	return ""
}

// entryLabel names one entry in an error message.
func entryLabel(entry *SourceExport) string {
	if entry == nil {
		return "(empty)"
	}

	if strings.TrimSpace(entry.URL) != "" {
		return fmt.Sprintf("%q", entry.URL)
	}

	return fmt.Sprintf("%q", entry.Title)
}

// toSourceExport maps one stored subscription onto its portable shape.
func toSourceExport(source *feed.Source, category string) *SourceExport {
	return &SourceExport{
		Title:         source.Title,
		URL:           source.URL,
		CURL:          source.CURL,
		Weight:        source.Weight,
		MaxFetchNum:   source.MaxFetchNum,
		Regex:         source.Regex,
		TitleExp:      source.TitleExp,
		URLExp:        source.URLExp,
		Redirect:      source.Redirect,
		Sort:          source.Sort,
		IsJSON:        source.IsJSON,
		JSONTitlePath: source.JsonTitlePath,
		JSONURLPath:   source.JsonURLPath,
		ParseType:     source.ParseType,
		AgentProvider: source.AgentProvider,
		AgentPrompt:   source.AgentPrompt,
		Category:      category,
		Enabled:       source.Enabled,
	}
}

// applySourceExport copies every configuration field of one entry onto a source
// record, leaving the id and the fetch health of an existing record untouched.
func applySourceExport(source *feed.Source, entry *SourceExport, categoryID int64) *feed.Source {
	source.Title = strings.TrimSpace(entry.Title)
	source.URL = strings.TrimSpace(entry.URL)
	source.CURL = entry.CURL
	source.Weight = entry.Weight
	source.MaxFetchNum = entry.MaxFetchNum
	source.Regex = entry.Regex
	source.TitleExp = entry.TitleExp
	source.URLExp = entry.URLExp
	source.Redirect = entry.Redirect
	source.Sort = entry.Sort
	source.IsJSON = entry.IsJSON
	source.JsonTitlePath = entry.JSONTitlePath
	source.JsonURLPath = entry.JSONURLPath
	source.ParseType = entry.ParseType
	source.AgentProvider = entry.AgentProvider
	source.AgentPrompt = entry.AgentPrompt
	source.CategoryID = categoryID
	source.Enabled = entry.Enabled

	return source
}
