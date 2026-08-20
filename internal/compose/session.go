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

// Package compose works out the configuration of a subscription that does not
// exist yet, by talking to the person who wants it.
//
// Adding a source means deciding which of four parse types a site needs and then
// writing the regex or the json path that goes with it. That is the step people
// stop at, and it is the same loop internal/diagnose already hands to an agent -
// read the bytes, try a candidate, look at what came out - only started from
// nothing instead of from a configuration that used to work.
//
// The difference that shapes this package is that it is a conversation. A repair
// has everything it needs the moment it starts; composing does not - which site,
// which part of it, how many articles - so the agent has to be able to ask, and
// the person has to be able to answer. Each turn is its own process, and what
// carries between them is the agent command line's own transcript.
//
// Like diagnose, nothing here can save. The agent hands over a proposal through
// propose_config; informer verifies it again on its own, and a person clicks the
// button that creates the subscription.
package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/mcp"
	"github.com/vogo/informer/internal/parsecfg"
)

// SessionFileName is the document one conversation is described by. It lives in
// a directory of its own that the parent process creates and removes, and the
// mcp child process reads it instead of opening the database.
//
// The directory outlives a single turn, because the conversation does: every
// turn launches a new child, and each one has to find the same session.
const SessionFileName = "session.json"

// ProposalFileName is where a child writes the configuration it settled on.
//
// It is a file rather than a shape parsed out of the reply text because a reply
// is prose: an answer that explains a regex and an answer that proposes one are
// not reliably told apart by looking for braces, and the regex would be retyped
// by the model on the way through. Here the parent reads exactly the string the
// trial ran with.
const ProposalFileName = "proposal.json"

// MCPConfigFileName is the mcp server document handed to the agent command line.
const MCPConfigFileName = "mcp.json"

// ServerName is how the mcp server is addressed. It is part of the tool names
// the agent sees - mcp__informer__try_parse - and is deliberately the same name
// diagnose uses, so the prefix is one string across the product.
const ServerName = "informer"

// ErrNoSession marks a directory that carries no readable session document.
var ErrNoSession = errors.New("compose session not found")

// ErrNoProposal marks a proposal document that carries no configuration.
var ErrNoProposal = errors.New("compose proposal carries no configuration")

// Session is what one composing conversation shares with its child processes.
//
// It is short because a conversation keeps its state where the conversation is -
// in the agent's transcript. What has to cross the process boundary is only the
// part the tools need to behave like informer itself.
type Session struct {
	// HTTPProxy is the proxy the parent uses for its own fetches, so the child's
	// fetches go out the same way. An empty value means direct.
	HTTPProxy string `json:"http_proxy"`

	// Dir is the run directory this session was read from. It is filled in by
	// ReadSession rather than stored, and is what propose_config writes into.
	Dir string `json:"-"`
}

// Proposal is the configuration a conversation settled on.
type Proposal struct {
	// Title is what the subscription should be called. It is the one field
	// outside the parse configuration the agent gets to fill in, because it is
	// the one a person would otherwise have to invent for a site they just
	// described in words.
	Title string `json:"title"`

	// Reason is why this configuration, in the user's language.
	Reason string `json:"reason"`

	// Changes is the parse configuration itself, against an empty draft.
	Changes *parsecfg.Changes `json:"changes"`
}

// WriteSession stores the session document inside dir, creating the directory.
func WriteSession(dir string, session *Session) error {
	if session == nil {
		return fmt.Errorf("%w: session is nil", ErrNoSession)
	}

	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return fmt.Errorf("create compose dir: %w", err)
	}

	encoded, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compose session: %w", err)
	}

	err = os.WriteFile(filepath.Join(dir, SessionFileName), encoded, 0o600)
	if err != nil {
		return fmt.Errorf("write compose session: %w", err)
	}

	return nil
}

// ReadSession loads the session document of a run directory.
func ReadSession(dir string) (*Session, error) {
	data, err := os.ReadFile(filepath.Join(dir, SessionFileName))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSession, err)
	}

	var session Session

	err = json.Unmarshal(data, &session)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSession, err)
	}

	session.Dir = dir

	return &session, nil
}

// WriteProposal stores the configuration a turn settled on, replacing whatever
// an earlier turn left there. Last one wins: the conversation moves forward, and
// the earlier proposals are still in the chat transcript a person is reading.
func WriteProposal(dir string, proposal *Proposal) error {
	if proposal == nil || proposal.Changes.IsEmpty() {
		return ErrNoProposal
	}

	encoded, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compose proposal: %w", err)
	}

	err = os.WriteFile(filepath.Join(dir, ProposalFileName), encoded, 0o600)
	if err != nil {
		return fmt.Errorf("write compose proposal: %w", err)
	}

	return nil
}

// ReadProposal loads the configuration the conversation has settled on so far,
// answering nil when it has settled on none. A turn that only asked a question
// is the normal case, not an error.
func ReadProposal(dir string) (*Proposal, error) {
	data, err := os.ReadFile(filepath.Join(dir, ProposalFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil //nolint:nilnil //no proposal yet is an answer, not a failure.
		}

		return nil, fmt.Errorf("read compose proposal: %w", err)
	}

	var proposal Proposal

	err = json.Unmarshal(data, &proposal)
	if err != nil {
		return nil, fmt.Errorf("decode compose proposal: %w", err)
	}

	if proposal.Changes.IsEmpty() {
		return nil, ErrNoProposal
	}

	return &proposal, nil
}

// WriteMCPConfig stores the mcp server document of a conversation and returns
// its path.
//
// command is the executable the agent should launch to reach the tools - the
// running informer binary itself - and args are what puts that binary into
// server mode for this run directory.
func WriteMCPConfig(dir, command string, args ...string) (string, error) {
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return "", fmt.Errorf("create compose dir: %w", err)
	}

	path := filepath.Join(dir, MCPConfigFileName)

	return path, mcp.WriteConfig(path, ServerName, command, args...)
}

// AllowedTools is the tool set a composing conversation may use, in the form the
// agent command line expects.
//
// The three server tools plus the two read only web tools. Searching is how an
// agent finds the feed address of a blog the user only named, which no amount of
// path probing reliably does; fetching a page through WebFetch is exploration
// too. What neither may do is settle the question - the verdict rests on
// fetch_content's bytes and on a trial that really parsed - and that half of the
// rule is stated in the prompt, which is the only place it can be.
func AllowedTools() string {
	return agent.DefaultAllowedTools + "," + mcp.QualifiedNames(ServerName, toolNames...)
}
