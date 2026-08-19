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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vogo/informer/internal/diagnose"
	"github.com/vogo/informer/internal/runlog"
	"github.com/vogo/informer/internal/service"
)

// DiagnoseLogEvent is the wails event every diagnosis progress line arrives on.
//
// It is separate from PreviewLogEvent so a diagnosis started from a card does
// not scroll its lines into a test fetch panel that happens to be open, and the
// frontend has to declare the same name; SourceDiagnose.vue is the other half.
const DiagnoseLogEvent = "informer:diagnose:log"

// diagnoseSinkFor is where one diagnosis reports to, or nil for a caller that
// asked for no reporting.
func diagnoseSinkFor(runID string) runlog.Sink {
	return runLogSinkFor(DiagnoseLogEvent, runID)
}

// ErrDiagnoseRunning marks a diagnosis issued while another one of this process
// is still in flight. One agent run at a time is deliberate: each one drives a
// command line for minutes and spends real api budget, and two at once is far
// more often a double click than an intention.
var ErrDiagnoseRunning = errors.New("a diagnosis is already running, wait for it to finish")

// ErrNoFixToApply marks an apply call carrying no proposal.
var ErrNoFixToApply = errors.New("there is no proposed fix to apply")

// FieldChangeDTO is one configuration field a diagnosis proposes to edit.
type FieldChangeDTO struct {
	// Field is the column name, the same name the settings form uses.
	Field string `json:"field"`

	// Old is the stored value, New the proposed one, both rendered as text.
	Old string `json:"old"`
	New string `json:"new"`
}

// DiagnoseVerificationDTO is informer's own re-parse of a proposed fix.
type DiagnoseVerificationDTO struct {
	// Ran says a verification parse happened at all; false when nothing was
	// proposed to verify.
	Ran bool `json:"ran"`

	// ArticleCount is how many articles the proposal parsed out.
	ArticleCount int `json:"articleCount"`

	// Samples are the first parsed articles, so the titles can be eyeballed
	// before the change is stored: a regex that matches the menu bar also
	// "works".
	Samples []*ArticleDTO `json:"samples"`

	// Error is why the verification failed, empty when it succeeded.
	Error string `json:"error"`
}

// DiagnoseReportDTO is what one diagnosis run tells the window.
type DiagnoseReportDTO struct {
	// SourceID is the subscription that was diagnosed.
	SourceID int64 `json:"sourceId"`

	// Fixed is informer's own verdict, the only one the apply button follows.
	Fixed bool `json:"fixed"`

	// AgentClaimedFixed is what the agent said about its own work, shown next
	// to Fixed so a disagreement is visible instead of silently resolved.
	AgentClaimedFixed bool `json:"agentClaimedFixed"`

	// Diagnosis explains what went wrong, Advice what to do when nothing could
	// be repaired.
	Diagnosis string `json:"diagnosis"`
	Advice    string `json:"advice"`

	// Diff is the proposed edit rendered for a person to read; empty when the
	// diagnosis proposes nothing.
	Diff []*FieldChangeDTO `json:"diff"`

	// Fix is the proposal encoded, to be handed straight back to ApplySourceFix.
	//
	// It travels as opaque text on purpose: the page shows Diff and decides
	// whether to apply, and exactly one place - the diagnose package - ever has
	// to know what the fields are called or which of them are booleans.
	Fix string `json:"fix"`

	// Verification is what informer's own re-parse produced.
	Verification *DiagnoseVerificationDTO `json:"verification"`
}

// DiagnoseSource asks the configured agent why a subscription stopped parsing.
//
// It writes nothing: the source, its health state and its stored error all come
// through untouched, and the returned proposal becomes real only when the user
// presses apply and ApplySourceFix is called with the returned fix.
//
// runID, when not empty, streams the run's progress to the window on
// DiagnoseLogEvent. A diagnosis takes minutes - the agent reads the page, tries
// a candidate, reads the result, tries again - so watching it is the difference
// between a diagnosis and a spinner.
func (a *App) DiagnoseSource(id int64, runID string) (*DiagnoseReportDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	if !a.diagnoseMu.TryLock() {
		return nil, ErrDiagnoseRunning
	}

	defer a.diagnoseMu.Unlock()

	report, err := a.svc.DiagnoseSource(context.Background(), id, diagnoseSinkFor(runID))
	if err != nil {
		return nil, err
	}

	dto := &DiagnoseReportDTO{
		SourceID:          report.SourceID,
		Fixed:             report.Fixed,
		AgentClaimedFixed: report.AgentClaimedFixed,
		Diagnosis:         report.Diagnosis,
		Advice:            report.Advice,
		Diff:              make([]*FieldChangeDTO, 0, len(report.Diff)),
		Verification:      toVerificationDTO(report.Verification),
	}

	for _, change := range report.Diff {
		dto.Diff = append(dto.Diff, &FieldChangeDTO{Field: change.Field, Old: change.Old, New: change.New})
	}

	if report.Changes != nil {
		encoded, marshalErr := json.Marshal(report.Changes)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode proposed fix: %w", marshalErr)
		}

		dto.Fix = string(encoded)
	}

	return dto, nil
}

// toVerificationDTO maps informer's own check into the frontend shape.
func toVerificationDTO(verification *service.DiagnoseVerification) *DiagnoseVerificationDTO {
	if verification == nil {
		return &DiagnoseVerificationDTO{Samples: []*ArticleDTO{}}
	}

	dto := &DiagnoseVerificationDTO{
		Ran:          verification.Ran,
		ArticleCount: verification.ArticleCount,
		Error:        verification.Error,
		Samples:      make([]*ArticleDTO, 0, len(verification.Samples)),
	}

	for _, article := range verification.Samples {
		dto.Samples = append(dto.Samples, &ArticleDTO{Title: article.Title, URL: article.URL})
	}

	return dto
}

// ApplySourceFix stores a diagnosis proposal, and is the only call of the whole
// diagnosis path that writes anything.
//
// fix is the opaque text a DiagnoseReportDTO carried; the page hands it back
// unchanged. The service re-validates the result and re-parses the repaired
// source, so a card stops claiming a failure the user just fixed.
//
// runID, when not empty, streams what was applied to the window on
// DiagnoseLogEvent, so the apply lands in the same panel the diagnosis did.
func (a *App) ApplySourceFix(id int64, fix, runID string) error {
	err := a.ready()
	if err != nil {
		return err
	}

	if fix == "" {
		return ErrNoFixToApply
	}

	var changes diagnose.Changes

	err = json.Unmarshal([]byte(fix), &changes)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoFixToApply, err)
	}

	return a.svc.ApplySourceFix(id, &changes, diagnoseSinkFor(runID))
}
