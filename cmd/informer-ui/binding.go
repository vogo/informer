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

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// ErrNotReady marks a binding call issued while the app startup failed. The
// frontend shows it like any other binding error and offers a restart.
var ErrNotReady = errors.New("informer did not finish starting up, see the startup error")

// ErrInformRunning marks a trigger issued while another inform run of this
// process is still in flight. The frontend shows it like any other binding
// error; the user simply tries again once the running push has finished.
var ErrInformRunning = errors.New("an inform run is already in progress, try again in a moment")

// SourceDTO is the flat subscription record the frontend sees. It carries
// every column a source owns but no persistence detail, so the wails binding
// can never turn the gorm model into the public desktop contract.
type SourceDTO struct {
	ID                int64  `json:"id"`
	Title             string `json:"title"`
	URL               string `json:"url"`
	CURL              string `json:"curl"`
	Weight            int64  `json:"weight"`
	MaxFetchNum       int    `json:"maxFetchNum"`
	Regex             string `json:"regex"`
	TitleExp          string `json:"titleExp"`
	URLExp            string `json:"urlExp"`
	Redirect          bool   `json:"redirect"`
	Sort              bool   `json:"sort"`
	IsJSON            bool   `json:"isJSON"`
	JSONTitlePath     string `json:"jsonTitlePath"`
	JSONURLPath       string `json:"jsonURLPath"`
	ParseType         string `json:"parseType"`
	AgentProvider     string `json:"agentProvider"`
	AgentPrompt       string `json:"agentPrompt"`
	CategoryID        int64  `json:"categoryId"`
	Enabled           bool   `json:"enabled"`
	Status            int    `json:"status"`
	ErrorInfo         string `json:"errorInfo"`
	ResolvedParseType string `json:"resolvedParseType"`
}

// SaveSourceRequest is the create and update payload of one subscription.
// A zero ID creates; a non zero ID updates that source.
type SaveSourceRequest struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	CURL          string `json:"curl"`
	Weight        int64  `json:"weight"`
	MaxFetchNum   int    `json:"maxFetchNum"`
	Regex         string `json:"regex"`
	TitleExp      string `json:"titleExp"`
	URLExp        string `json:"urlExp"`
	Redirect      bool   `json:"redirect"`
	Sort          bool   `json:"sort"`
	IsJSON        bool   `json:"isJSON"`
	JSONTitlePath string `json:"jsonTitlePath"`
	JSONURLPath   string `json:"jsonURLPath"`
	ParseType     string `json:"parseType"`
	AgentProvider string `json:"agentProvider"`
	AgentPrompt   string `json:"agentPrompt"`
	CategoryID    int64  `json:"categoryId"`
	Enabled       bool   `json:"enabled"`
}

// ArticleDTO is one preview candidate: exactly what the test fetch panel shows.
type ArticleDTO struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// toSourceDTO maps one persistence model into the frontend shape.
func toSourceDTO(source *feed.Source) *SourceDTO {
	return &SourceDTO{
		ID:                source.ID,
		Title:             source.Title,
		URL:               source.URL,
		CURL:              source.CURL,
		Weight:            source.Weight,
		MaxFetchNum:       source.MaxFetchNum,
		Regex:             source.Regex,
		TitleExp:          source.TitleExp,
		URLExp:            source.URLExp,
		Redirect:          source.Redirect,
		Sort:              source.Sort,
		IsJSON:            source.IsJSON,
		JSONTitlePath:     source.JsonTitlePath,
		JSONURLPath:       source.JsonURLPath,
		ParseType:         source.ParseType,
		AgentProvider:     source.AgentProvider,
		AgentPrompt:       source.AgentPrompt,
		CategoryID:        source.CategoryID,
		Enabled:           source.Enabled,
		Status:            source.Status,
		ErrorInfo:         source.ErrorInfo,
		ResolvedParseType: source.ResolveParseType(),
	}
}

// Enabled states a subscription listing can be restricted to. The tri state is
// spelled out instead of sent as a nullable boolean, because the sidebar has to
// distinguish "no opinion" from "explicitly disabled" across the json boundary.
const (
	EnabledStateOn  = "enabled"
	EnabledStateOff = "disabled"
)

// SourceQueryRequest is the filter snapshot the subscription page submits. Every
// field is optional and an empty one drops that dimension; the filled ones are
// combined with AND. An unknown value is a reported error rather than a
// silently unfiltered list, so a stale frontend cannot claim a filter took hold.
type SourceQueryRequest struct {
	// CategoryID restricts the listing to one category, zero means every category.
	CategoryID int64 `json:"categoryId"`

	// ParseType is one of SupportedParseTypes and matches the resolved type -
	// the same value SourceDTO.ResolvedParseType carries and the card shows.
	ParseType string `json:"parseType"`

	// FetchStatus is "normal", "error" or "unfetched".
	FetchStatus string `json:"fetchStatus"`

	// EnabledState is EnabledStateOn or EnabledStateOff.
	EnabledState string `json:"enabledState"`
}

// SupportedParseTypes returns the parse types a subscription can be filtered by,
// in presentation order. It answers from the parser set itself, so a sidebar
// built from this list offers exactly the types the query accepts. It reads no
// database and therefore stays available while the app reports a startup error.
func (a *App) SupportedParseTypes() []string {
	return feed.LegalParseTypes()
}

// SupportedAgentProviders returns the agent providers a subscription of the agent
// parse type can be pinned to, in presentation order. Like SupportedParseTypes it
// answers from the domain set itself and reads no database, so the subscription
// form can offer exactly the values the service accepts.
func (a *App) SupportedAgentProviders() []string {
	return agent.LegalProviders()
}

// ListSources returns the subscriptions matching the filter snapshot, ordered by
// id. A nil request lists every subscription. The result is never nil so an
// empty database - or a filter nothing matches - renders as an empty list,
// not a failure.
func (a *App) ListSources(req *SourceQueryRequest) ([]*SourceDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	query, err := toSourceQuery(req)
	if err != nil {
		return nil, err
	}

	sources, err := a.svc.AllSources(query)
	if err != nil {
		return nil, err
	}

	dtos := make([]*SourceDTO, 0, len(sources))
	for _, source := range sources {
		dtos = append(dtos, toSourceDTO(source))
	}

	return dtos, nil
}

// toSourceQuery maps the frontend filter snapshot onto the service query. The
// parse type and the fetch status pass through untouched: the service owns their
// vocabulary and rejects an unknown value there, in one place.
func toSourceQuery(req *SourceQueryRequest) (service.SourceQuery, error) {
	if req == nil {
		return service.SourceQuery{}, nil
	}

	query := service.SourceQuery{
		CategoryID:  req.CategoryID,
		ParseType:   req.ParseType,
		FetchStatus: req.FetchStatus,
	}

	switch req.EnabledState {
	case "":
	case EnabledStateOn:
		enabled := true
		query.Enabled = &enabled
	case EnabledStateOff:
		disabled := false
		query.Enabled = &disabled
	default:
		return service.SourceQuery{},
			fmt.Errorf("%w: unknown enabled state %q", service.ErrInvalidArgument, req.EnabledState)
	}

	return query, nil
}

// CreateSource stores a new subscription and returns the stored record.
// Validation, the default category and the default enabled state stay the
// service's decision; the binding only maps and reports.
func (a *App) CreateSource(req *SaveSourceRequest) (*SourceDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return nil, err
	}

	source := applyRequest(&feed.Source{}, req)
	source.ID = 0

	err = a.svc.CreateSource(source)
	if err != nil {
		return nil, err
	}

	return toSourceDTO(source), nil
}

// UpdateSource replaces the editable fields of one subscription. Fields the
// form does not show - status and error info - are merged back from the
// stored record, so an edit never overwrites them with zero values.
func (a *App) UpdateSource(req *SaveSourceRequest) (*SourceDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return nil, err
	}

	stored, err := a.svc.GetSource(req.ID)
	if err != nil {
		return nil, err
	}

	source := applyRequest(stored, req)
	source.Status = stored.Status
	source.ErrorInfo = stored.ErrorInfo

	err = a.svc.UpdateSource(source)
	if err != nil {
		return nil, err
	}

	return toSourceDTO(source), nil
}

// DeleteSource removes one subscription; the frontend confirms before calling.
func (a *App) DeleteSource(id int64) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.DeleteSource(id)
}

// SetSourceEnabled toggles one subscription without touching any other field.
func (a *App) SetSourceEnabled(id int64, enabled bool) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SetSourceEnabled(id, enabled)
}

// PreviewSource runs one real fetch and parse of a stored subscription and
// returns the candidate articles. It writes nothing - not the source, not
// articles, not health state - and a disabled source previews just the same.
func (a *App) PreviewSource(id int64) ([]*ArticleDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	articles, err := a.svc.Preview(id)
	if err != nil {
		return nil, err
	}

	dtos := make([]*ArticleDTO, 0, len(articles))
	for _, article := range articles {
		dtos = append(dtos, &ArticleDTO{Title: article.Title, URL: article.URL})
	}

	return dtos, nil
}

// applyRequest copies every editable request field onto one source record.
func applyRequest(source *feed.Source, req *SaveSourceRequest) *feed.Source {
	source.ID = req.ID
	source.Title = req.Title
	source.URL = req.URL
	source.CURL = req.CURL
	source.Weight = req.Weight
	source.MaxFetchNum = req.MaxFetchNum
	source.Regex = req.Regex
	source.TitleExp = req.TitleExp
	source.URLExp = req.URLExp
	source.Redirect = req.Redirect
	source.Sort = req.Sort
	source.IsJSON = req.IsJSON
	source.JsonTitlePath = req.JSONTitlePath
	source.JsonURLPath = req.JSONURLPath
	source.ParseType = req.ParseType
	source.AgentProvider = req.AgentProvider
	source.AgentPrompt = req.AgentPrompt
	source.CategoryID = req.CategoryID
	source.Enabled = req.Enabled

	return source
}
