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

// Package diagnose repairs a subscription whose parse stopped working.
//
// A regex source breaks when the page it reads changes shape, and the fix is
// always the same shallow edit - a different capture group, a moved path, a feed
// that now lives at another address. What makes it tedious is that finding the
// edit means looking at the bytes informer actually received and trying a
// candidate against them, over and over.
//
// This package hands exactly that loop to an agent, and nothing else. It offers
// three tools - read the configuration, read the fetched document, try a
// candidate configuration - and it deliberately offers no way to save. The agent
// answers with a proposed change; informer re-verifies it on its own and a person
// decides whether it is applied. An agent that could write to the database would
// be one bad regex away from turning a source that half worked into one that
// does not work at all, with the original already overwritten.
package diagnose

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/vogo/informer/internal/feed"
)

// Field names, shared with the tool schemas so a column is spelled once.
const (
	fieldURL           = "url"
	fieldCURL          = "curl"
	fieldParseType     = "parse_type"
	fieldRegex         = "regex"
	fieldTitleExp      = "title_exp"
	fieldURLExp        = "url_exp"
	fieldIsJSON        = "is_json"
	fieldJSONTitlePath = "json_title_path"
	fieldJSONURLPath   = "json_url_path"
	fieldAgentProvider = "agent_provider"
	fieldAgentPrompt   = "agent_prompt"
	fieldRedirect      = "redirect"
)

// Changes is the set of parse affecting fields a diagnosis proposes to edit.
//
// Every field is a pointer so that "not mentioned" and "set to empty" stay
// different answers: clearing a regex is a legitimate fix when a source should
// become a plain feed again, and it must not look like an omission.
//
// The set is deliberately narrow. Title, category, weight, fetch limit and the
// enabled flag are absent because none of them can be the reason a parse fails,
// and a repair that quietly moved a source to another category would be a
// surprise no diagnosis is worth.
type Changes struct {
	URL           *string `json:"url,omitempty"`
	CURL          *string `json:"curl,omitempty"`
	ParseType     *string `json:"parse_type,omitempty"`
	Regex         *string `json:"regex,omitempty"`
	TitleExp      *string `json:"title_exp,omitempty"`
	URLExp        *string `json:"url_exp,omitempty"`
	IsJSON        *bool   `json:"is_json,omitempty"`
	JSONTitlePath *string `json:"json_title_path,omitempty"`
	JSONURLPath   *string `json:"json_url_path,omitempty"`
	AgentProvider *string `json:"agent_provider,omitempty"`
	AgentPrompt   *string `json:"agent_prompt,omitempty"`
	Redirect      *bool   `json:"redirect,omitempty"`
}

// FieldChange is one field a diagnosis proposes to edit, in the shape a person
// reads it: what it is now and what it would become.
type FieldChange struct {
	// Field is the json name of the column, the same name the agent used.
	Field string `json:"field"`

	// Old is the stored value rendered as text.
	Old string `json:"old"`

	// New is the proposed value rendered as text.
	New string `json:"new"`
}

// IsEmpty reports whether the diagnosis proposes no edit at all.
func (c *Changes) IsEmpty() bool {
	return c == nil || len(c.fields()) == 0
}

// field is one entry of the field table below: how to read the proposal, how to
// read the stored value, and how to write the proposal into a source.
type field struct {
	name    string
	stringP func(*Changes) *string
	boolP   func(*Changes) *bool
	getStr  func(*feed.Source) string
	getBool func(*feed.Source) bool
	setStr  func(*feed.Source, string)
	setBool func(*feed.Source, bool)
}

// fieldTable is the single place a repairable column is declared. Apply, the
// diff and the prompt's field list all read from it, so a column added here
// needs no matching edit anywhere else.
//
//nolint:gochecknoglobals //a constant table, built once.
var fieldTable = []field{
	{
		name:    fieldURL,
		stringP: func(c *Changes) *string { return c.URL },
		getStr:  func(s *feed.Source) string { return s.URL },
		setStr:  func(s *feed.Source, v string) { s.URL = v },
	},
	{
		name:    fieldCURL,
		stringP: func(c *Changes) *string { return c.CURL },
		getStr:  func(s *feed.Source) string { return s.CURL },
		setStr:  func(s *feed.Source, v string) { s.CURL = v },
	},
	{
		name:    fieldParseType,
		stringP: func(c *Changes) *string { return c.ParseType },
		getStr:  func(s *feed.Source) string { return s.ParseType },
		setStr:  func(s *feed.Source, v string) { s.ParseType = v },
	},
	{
		name:    fieldRegex,
		stringP: func(c *Changes) *string { return c.Regex },
		getStr:  func(s *feed.Source) string { return s.Regex },
		setStr:  func(s *feed.Source, v string) { s.Regex = v },
	},
	{
		name:    fieldTitleExp,
		stringP: func(c *Changes) *string { return c.TitleExp },
		getStr:  func(s *feed.Source) string { return s.TitleExp },
		setStr:  func(s *feed.Source, v string) { s.TitleExp = v },
	},
	{
		name:    fieldURLExp,
		stringP: func(c *Changes) *string { return c.URLExp },
		getStr:  func(s *feed.Source) string { return s.URLExp },
		setStr:  func(s *feed.Source, v string) { s.URLExp = v },
	},
	{
		name:    fieldIsJSON,
		boolP:   func(c *Changes) *bool { return c.IsJSON },
		getBool: func(s *feed.Source) bool { return s.IsJSON },
		setBool: func(s *feed.Source, v bool) { s.IsJSON = v },
	},
	{
		name:    fieldJSONTitlePath,
		stringP: func(c *Changes) *string { return c.JSONTitlePath },
		getStr:  func(s *feed.Source) string { return s.JsonTitlePath },
		setStr:  func(s *feed.Source, v string) { s.JsonTitlePath = v },
	},
	{
		name:    fieldJSONURLPath,
		stringP: func(c *Changes) *string { return c.JSONURLPath },
		getStr:  func(s *feed.Source) string { return s.JsonURLPath },
		setStr:  func(s *feed.Source, v string) { s.JsonURLPath = v },
	},
	{
		name:    fieldAgentProvider,
		stringP: func(c *Changes) *string { return c.AgentProvider },
		getStr:  func(s *feed.Source) string { return s.AgentProvider },
		setStr:  func(s *feed.Source, v string) { s.AgentProvider = v },
	},
	{
		name:    fieldAgentPrompt,
		stringP: func(c *Changes) *string { return c.AgentPrompt },
		getStr:  func(s *feed.Source) string { return s.AgentPrompt },
		setStr:  func(s *feed.Source, v string) { s.AgentPrompt = v },
	},
	{
		name:    fieldRedirect,
		boolP:   func(c *Changes) *bool { return c.Redirect },
		getBool: func(s *feed.Source) bool { return s.Redirect },
		setBool: func(s *feed.Source, v bool) { s.Redirect = v },
	},
}

// RepairableFields lists the columns a diagnosis may edit, in declaration order.
// The prompt states this list, so what the agent is allowed to touch and what it
// is told about can never drift apart.
func RepairableFields() []string {
	names := make([]string, 0, len(fieldTable))
	for _, entry := range fieldTable {
		names = append(names, entry.name)
	}

	return names
}

// Apply returns a copy of the source with the proposed edits written into it.
// The stored record is never touched: the copy is what a verification run parses
// with, and only an explicit save by a person turns it into the stored one.
func (c *Changes) Apply(source *feed.Source) *feed.Source {
	patched := &feed.Source{}
	if source != nil {
		*patched = *source
	}

	for _, entry := range c.fields() {
		if entry.stringP != nil {
			entry.setStr(patched, strings.TrimSpace(*entry.stringP(c)))

			continue
		}

		entry.setBool(patched, *entry.boolP(c))
	}

	return patched
}

// Diff renders the proposed edits against a source, dropping the fields whose
// proposed value is what is already stored. An agent that echoes the whole
// configuration back then shows up as the one line it actually changed.
func (c *Changes) Diff(source *feed.Source) []FieldChange {
	diff := make([]FieldChange, 0, len(fieldTable))

	for _, entry := range c.fields() {
		change := FieldChange{
			Field: entry.name,
			Old:   entry.storedText(source),
			New:   entry.proposedText(c),
		}
		if change.Old == change.New {
			continue
		}

		diff = append(diff, change)
	}

	return diff
}

// storedText renders what the source holds in this field today.
func (f field) storedText(source *feed.Source) string {
	if f.stringP != nil {
		return f.getStr(source)
	}

	return strconv.FormatBool(f.getBool(source))
}

// proposedText renders what the diagnosis would put there instead.
func (f field) proposedText(changes *Changes) string {
	if f.stringP != nil {
		return strings.TrimSpace(*f.stringP(changes))
	}

	return strconv.FormatBool(*f.boolP(changes))
}

// Effective returns the edits that would really change something, so a proposal
// that only restates the stored configuration is recognized as the no-op it is.
//
// It is built by dropping the unchanged keys from the encoded proposal rather
// than by copying field by field: the json names are already the single
// vocabulary this package speaks, and a second hand written mapping is one more
// place a newly repairable column could be forgotten.
func (c *Changes) Effective(source *feed.Source) *Changes {
	diff := c.Diff(source)
	if len(diff) == 0 {
		return nil
	}

	encoded, err := json.Marshal(c)
	if err != nil {
		// Changes is a flat struct of strings and bools; this cannot fail, and
		// the whole proposal is the honest fallback if it ever did.
		return c
	}

	var raw map[string]json.RawMessage

	err = json.Unmarshal(encoded, &raw)
	if err != nil {
		return c
	}

	changed := make(map[string]bool, len(diff))
	for _, entry := range diff {
		changed[entry.Field] = true
	}

	for name := range raw {
		if !changed[name] {
			delete(raw, name)
		}
	}

	kept := &Changes{}

	trimmed, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(trimmed, kept) != nil {
		return c
	}

	return kept
}

// fields returns the proposed edits in declaration order, skipping the fields
// the diagnosis did not mention.
func (c *Changes) fields() []field {
	if c == nil {
		return nil
	}

	proposed := make([]field, 0, len(fieldTable))

	for _, entry := range fieldTable {
		if entry.stringP != nil && entry.stringP(c) != nil {
			proposed = append(proposed, entry)

			continue
		}

		if entry.boolP != nil && entry.boolP(c) != nil {
			proposed = append(proposed, entry)
		}
	}

	return proposed
}
