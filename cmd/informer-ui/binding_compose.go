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

	"github.com/vogo/informer/internal/parsecfg"
	"github.com/vogo/informer/internal/runlog"
	"github.com/vogo/informer/internal/service"
)

// ComposeLogEvent is the wails event every composing progress line arrives on.
//
// It is its own name so a conversation's tool calls do not scroll into the test
// fetch panel or the diagnosis panel that happens to be open; SourceCompose.vue
// declares the same string.
const ComposeLogEvent = "informer:compose:log"

// composeSinkFor is where one turn reports to, or nil for a caller that asked
// for no reporting.
func composeSinkFor(runID string) runlog.Sink {
	return runLogSinkFor(ComposeLogEvent, runID)
}

// ErrComposeRunning marks a turn issued while another agent run of this process
// is still in flight. It shares the gate with a diagnosis: both drive a command
// line for minutes, and two at once is what the gate exists to prevent.
var ErrComposeRunning = errors.New("an AI run is already in progress, wait for it to finish")

// ErrNoConfigToSave marks a save call carrying no proposal.
var ErrNoConfigToSave = errors.New("there is no proposed configuration to save")

// ComposeProposalDTO is a configuration a conversation arrived at, after
// informer checked it itself.
type ComposeProposalDTO struct {
	// Title is what the subscription would be called.
	Title string `json:"title"`

	// Reason is the agent's one line justification, in the user's language.
	Reason string `json:"reason"`

	// ParseType is the type the configuration resolves to, so the page can say
	// so plainly instead of making the user read it out of the field list.
	ParseType string `json:"parseType"`

	// Fields is the configuration rendered field by field, for reading. The
	// shape is shared with a diagnosis; here Old is always empty, because a
	// subscription that does not exist yet has no previous value.
	Fields []*FieldChangeDTO `json:"fields"`

	// Config is the proposal encoded, to be handed straight back to
	// CreateSourceFromCompose.
	//
	// It travels as opaque text for the same reason a diagnosis's fix does: the
	// page shows Fields and decides whether to save, and exactly one package
	// ever has to know what the columns are called.
	Config string `json:"config"`

	// Verification is what informer's own parse with this configuration
	// produced.
	Verification *DiagnoseVerificationDTO `json:"verification"`

	// Savable is informer's own verdict, the only one the save button follows.
	Savable bool `json:"savable"`
}

// ComposeReplyDTO is one turn of a composing conversation.
type ComposeReplyDTO struct {
	// SessionID is the conversation this turn belongs to.
	SessionID string `json:"sessionId"`

	// Message is what the agent said, as markdown, meant for a person to read.
	Message string `json:"message"`

	// Turns is how many turns have completed, so the page can warn as the
	// conversation approaches the length where starting over is better.
	Turns int `json:"turns"`

	// Proposal is the configuration this turn arrived at, absent when the turn
	// only asked a question or restated one already shown.
	Proposal *ComposeProposalDTO `json:"proposal"`
}

// StartCompose opens a conversation about a subscription that does not exist
// yet, and returns its id.
//
// No agent runs here: the conversation begins with the user's first message.
// At most one conversation is alive at a time, so starting a second ends the
// first and removes what it left on disk.
func (a *App) StartCompose() (string, error) {
	err := a.ready()
	if err != nil {
		return "", err
	}

	return a.svc.StartCompose()
}

// ComposeChat hands one message to the conversation and returns what came back.
//
// It writes nothing. The agent may record a configuration, informer parses with
// it to find out whether it really works, and the subscription is created only
// by an explicit CreateSourceFromCompose.
//
// runID, when not empty, streams the turn's progress to the window on
// ComposeLogEvent - every page the agent read, every candidate it tried - which
// for a turn measured in minutes is the difference between a conversation and a
// spinner.
func (a *App) ComposeChat(sessionID, message, runID string) (*ComposeReplyDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	if !a.agentMu.TryLock() {
		return nil, ErrComposeRunning
	}

	defer a.agentMu.Unlock()

	reply, err := a.svc.ComposeChat(context.Background(), sessionID, message, composeSinkFor(runID))
	if err != nil {
		return nil, err
	}

	dto := &ComposeReplyDTO{
		SessionID: reply.SessionID,
		Message:   reply.Message,
		Turns:     reply.Turns,
		Proposal:  nil,
	}

	dto.Proposal, err = toComposeProposalDTO(reply.Proposal)
	if err != nil {
		return nil, err
	}

	return dto, nil
}

// CloseCompose ends a conversation and removes its run directory. Closing one
// that is already gone is not an error: the modal closes for many reasons, and
// the page calls this on every one of them.
func (a *App) CloseCompose(sessionID string) error {
	err := a.ready()
	if err != nil {
		return err
	}

	a.svc.CloseCompose(sessionID)

	return nil
}

// CreateSourceFromCompose creates the subscription a proposal describes, and is
// the only call of the whole composing path that writes anything.
//
// config is the opaque text a ComposeProposalDTO carried; the page hands it back
// unchanged. It carries no session id on purpose: a conversation can produce
// several proposals and the save button belongs to one of them, so what is saved
// is the one the person was looking at rather than the newest one on the server.
func (a *App) CreateSourceFromCompose(config, title string, categoryID int64) (*SourceDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	if config == "" {
		return nil, ErrNoConfigToSave
	}

	var changes parsecfg.Changes

	err = json.Unmarshal([]byte(config), &changes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoConfigToSave, err)
	}

	source, err := a.svc.CreateSourceFromCompose(&changes, title, categoryID)
	if err != nil {
		return nil, err
	}

	return toSourceDTO(source), nil
}

// toComposeProposalDTO maps a checked proposal into the frontend shape.
func toComposeProposalDTO(proposal *service.ComposeProposal) (*ComposeProposalDTO, error) {
	if proposal == nil {
		return nil, nil //nolint:nilnil //a turn that only asked a question proposes nothing.
	}

	encoded, err := json.Marshal(proposal.Changes)
	if err != nil {
		return nil, fmt.Errorf("encode proposed configuration: %w", err)
	}

	dto := &ComposeProposalDTO{
		Title:        proposal.Title,
		Reason:       proposal.Reason,
		ParseType:    proposal.ParseType,
		Fields:       make([]*FieldChangeDTO, 0, len(proposal.Fields)),
		Config:       string(encoded),
		Verification: toVerificationDTO(proposal.Verification),
		Savable:      proposal.Savable,
	}

	for _, field := range proposal.Fields {
		dto.Fields = append(dto.Fields, &FieldChangeDTO{Field: field.Field, Old: field.Old, New: field.New})
	}

	return dto, nil
}
