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
	"context"
	"fmt"

	"github.com/vogo/informer/internal/resultprocess"
	"github.com/vogo/informer/internal/service"
)

// ResultProcessingConfigDTO is the editable final processing section.
type ResultProcessingConfigDTO struct {
	Enabled bool   `json:"enabled"`
	Prompt  string `json:"prompt"`
}

// ProcessingDecisionDTO explains one candidate's inclusion or exclusion.
type ProcessingDecisionDTO struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Selected bool   `json:"selected"`
	Reason   string `json:"reason"`
}

// ProcessingPreviewDTO is a read-only final-processing trial.
type ProcessingPreviewDTO struct {
	Original  string                  `json:"original"`
	Content   string                  `json:"content"`
	Action    string                  `json:"action"`
	Summary   string                  `json:"summary"`
	Decisions []ProcessingDecisionDTO `json:"decisions"`
}

// SaveResultProcessing stores the switch and instruction independently of other settings.
func (a *App) SaveResultProcessing(req *ResultProcessingConfigDTO) error {
	if req == nil {
		return fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}
	readyErr := a.ready()
	if readyErr != nil {
		return readyErr
	}

	return a.svc.SaveResultProcessingConfig(&resultprocess.Config{Enabled: req.Enabled, Prompt: req.Prompt})
}

// PreviewResultProcessing tries the current instruction against existing candidates,
// without saving the form, fetching articles or notifying any target.
func (a *App) PreviewResultProcessing(prompt string) (*ProcessingPreviewDTO, error) {
	readyErr := a.ready()
	if readyErr != nil {
		return nil, readyErr
	}
	if !a.agentMu.TryLock() {
		return nil, ErrDiagnoseRunning
	}
	defer a.agentMu.Unlock()
	preview, err := a.svc.PreviewResultProcessing(context.Background(), prompt)
	if err != nil {
		return nil, err
	}
	report := preview.Report
	titles := make(map[int64]string, len(preview.Candidates))
	for _, article := range preview.Candidates {
		titles[article.ID] = article.Title
	}
	dto := &ProcessingPreviewDTO{
		Original: preview.Original, Content: report.Content, Action: report.Action, Summary: report.Summary,
		Decisions: make([]ProcessingDecisionDTO, 0, len(preview.Candidates)),
	}
	for _, decision := range report.Articles {
		dto.Decisions = append(dto.Decisions, ProcessingDecisionDTO{ID: decision.ID, Title: titles[decision.ID], Selected: true, Reason: decision.Reason})
	}
	for _, decision := range report.Rejected {
		dto.Decisions = append(dto.Decisions, ProcessingDecisionDTO{ID: decision.ID, Title: titles[decision.ID], Reason: decision.Reason})
	}

	return dto, nil
}
