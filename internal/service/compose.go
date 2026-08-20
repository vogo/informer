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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/compose"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/parsecfg"
	"github.com/vogo/informer/internal/runlog"
)

// Bounds of one composing conversation.
const (
	// ComposeTurnTimeoutSeconds is the floor of one turn's time budget.
	//
	// A turn is not a fetch: the agent reads the page in windows, tries a
	// candidate, reads the result and tries again, so the configured fetch
	// timeout - five minutes by default - would cut it off halfway. The floor is
	// lower than a diagnosis's because a person is sitting in front of this one,
	// but a configured budget above it is still honored.
	ComposeTurnTimeoutSeconds = 600

	// composeMaxTurns bounds one conversation.
	//
	// Every turn resends the whole transcript, so the cost of a conversation
	// grows with its length rather than with its last message. A conversation
	// that has not found a working configuration in this many turns is one where
	// starting over states the problem better than continuing.
	composeMaxTurns = 12
)

// composeIdleTimeout is how long a conversation nobody is talking to is kept.
//
// It is a variable so a test can shorten it. The window closing is supposed to
// end a conversation, and normally does; this is what covers the window that was
// closed by killing the process.
var composeIdleTimeout = 30 * time.Minute //nolint:gochecknoglobals //shortened by tests.

// ComposeSession is one conversation about a subscription that does not exist
// yet, together with the run directory its tools read.
type ComposeSession struct {
	id    string
	dir   string
	agent *agent.Session

	// mu guards the three fields below. The idle timer fires on its own
	// goroutine and a turn runs for minutes, so closing a conversation reads
	// what a turn is writing; the lock is held for a field access, never across
	// the run.
	mu sync.Mutex

	// seen is the proposal already reported to the caller, encoded. The proposal
	// document survives the turn that wrote it, so without this a conversation
	// that went on chatting would keep re-announcing the same configuration.
	seen string

	idle   *time.Timer
	cancel context.CancelFunc
}

// ComposeReply is one turn of a composing conversation.
type ComposeReply struct {
	// SessionID is the conversation this turn belongs to.
	SessionID string `json:"session_id"`

	// Message is what the agent said, as markdown, meant to be read by a person.
	Message string `json:"message"`

	// Turns is how many turns of this conversation have completed.
	Turns int `json:"turns"`

	// Proposal is the configuration this turn arrived at, absent when the turn
	// only asked a question or restated one already reported.
	Proposal *ComposeProposal `json:"proposal"`
}

// ComposeProposal is a configuration the conversation arrived at, after informer
// checked it itself.
type ComposeProposal struct {
	// Title is what the subscription would be called.
	Title string `json:"title"`

	// Reason is the agent's one line justification, in the user's language.
	Reason string `json:"reason"`

	// ParseType is the type the configuration resolves to.
	ParseType string `json:"parse_type"`

	// Changes is the configuration itself, against an empty draft.
	Changes *parsecfg.Changes `json:"changes"`

	// Fields is the same configuration rendered field by field, for reading.
	Fields []parsecfg.FieldChange `json:"fields"`

	// Verification is informer's own parse with this configuration.
	Verification *ParseVerification `json:"verification"`

	// Savable is informer's own verdict, not the agent's: whether the desktop
	// should offer to create the subscription from this.
	Savable bool `json:"savable"`
}

// StartCompose opens a conversation and returns its id.
//
// At most one is alive per service. The modal that drives it is singular, so a
// second one starting means the first is gone; closing it here is what keeps a
// window that was force quit from leaving a run directory behind forever.
func (s *Service) StartCompose() (string, error) {
	dir, err := os.MkdirTemp("", "informer-compose-")
	if err != nil {
		return "", fmt.Errorf("create compose dir: %w", err)
	}

	document := &compose.Session{}

	proxy, proxyErr := s.readHTTPProxy()
	if proxyErr == nil {
		document.HTTPProxy = proxy
	}

	err = compose.WriteSession(dir, document)
	if err != nil {
		_ = os.RemoveAll(dir)

		return "", err
	}

	config, err := s.composeAgentConfig(dir)
	if err != nil {
		_ = os.RemoveAll(dir)

		return "", err
	}

	session := &ComposeSession{ //nolint:exhaustruct //the timer and cancel are armed below.
		id:    filepath.Base(dir),
		dir:   dir,
		agent: agent.NewSession(config),
	}
	session.idle = time.AfterFunc(composeIdleTimeout, func() { s.CloseCompose(session.id) })

	s.composeMu.Lock()
	previous := s.compose
	s.compose = session
	s.composeMu.Unlock()

	discardComposeSession(previous)

	return session.id, nil
}

// ComposeChat hands one message to the conversation and returns what came back.
//
// Nothing is written to the database here either. The agent may record a
// proposal, informer parses with it to see whether it really works, and the
// subscription is created only by an explicit CreateSourceFromCompose.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) ComposeChat(ctx context.Context, sessionID, message string,
	sink runlog.Sink,
) (*ComposeReply, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, fmt.Errorf("%w: message is empty", ErrInvalidArgument)
	}

	session, err := s.composeSession(sessionID)
	if err != nil {
		return nil, err
	}

	turns := session.agent.Turns()
	if turns >= composeMaxTurns {
		return nil, fmt.Errorf("%w: 对话已经进行了 %d 轮，请关闭后重新开始", ErrInvalidArgument, turns)
	}

	s.ApplyHTTPProxy()

	prompt := compose.TurnPrompt(message)
	if turns == 0 {
		prompt = compose.OpeningPrompt(message)
	}

	turnCtx, cancel := context.WithCancel(ctx)
	session.arm(cancel)

	defer cancel()

	runlog.Infof(sink, "第 %d 轮开始", turns+1)

	raw, err := session.agent.Send(turnCtx, prompt, agentObserver(sink))
	if err != nil {
		runlog.Errorf(sink, "这一轮失败了：%v", err)

		return nil, fmt.Errorf("compose turn: %w", err)
	}

	session.touch()

	reply := &ComposeReply{
		SessionID: session.id,
		Message:   strings.TrimSpace(raw),
		Turns:     session.agent.Turns(),
		Proposal:  nil,
	}

	//nolint:contextcheck //the verification parse runs under the http client's own deadline.
	reply.Proposal = s.composeProposal(session, raw, sink)

	return reply, nil
}

// CloseCompose ends a conversation and removes its run directory. Closing one
// that is already gone is not an error: the window closes for many reasons.
func (s *Service) CloseCompose(sessionID string) {
	s.composeMu.Lock()

	session := s.compose
	if session == nil || session.id != sessionID {
		s.composeMu.Unlock()

		return
	}

	s.compose = nil
	s.composeMu.Unlock()

	discardComposeSession(session)
}

// CreateSourceFromCompose creates the subscription a proposal describes.
//
// It takes the configuration rather than a session id on purpose. A conversation
// can produce several proposals and the save button belongs to one of them, and
// a person who left the modal open past the idle timeout should still be able to
// save what they were looking at. The configuration is re-validated here rather
// than trusted, exactly as an applied repair is.
func (s *Service) CreateSourceFromCompose(changes *parsecfg.Changes, title string,
	categoryID int64,
) (*feed.Source, error) {
	if changes.IsEmpty() {
		return nil, fmt.Errorf("%w: the proposal sets no field", ErrInvalidArgument)
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is empty", ErrInvalidArgument)
	}

	source := changes.Apply(&feed.Source{})
	source.Title = title
	source.CategoryID = categoryID

	err := s.CreateSource(source)
	if err != nil {
		return nil, err
	}

	return source, nil
}

// composeSession looks up a live conversation.
func (s *Service) composeSession(sessionID string) (*ComposeSession, error) {
	s.composeMu.Lock()
	defer s.composeMu.Unlock()

	if s.compose == nil || s.compose.id != sessionID {
		return nil, fmt.Errorf("%w: compose session %q", ErrNotFound, sessionID)
	}

	return s.compose, nil
}

// composeAgentConfig prepares the agent configuration every turn runs with: the
// mcp document the command line loads, the rules restated on each turn, the tool
// set a conversation needs, and a turn's own time budget.
func (s *Service) composeAgentConfig(dir string) (*agent.Config, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate informer executable for the compose tools: %w", err)
	}

	mcpPath, err := compose.WriteMCPConfig(dir, executable, compose.ServeCommand, "--dir", dir)
	if err != nil {
		return nil, err
	}

	config := s.agentConfig()
	config.MCPConfigPath = mcpPath
	config.AllowedTools = compose.AllowedTools()
	config.AppendSystemPrompt = compose.SystemRules()

	if config.TimeoutSeconds < ComposeTurnTimeoutSeconds {
		config.TimeoutSeconds = ComposeTurnTimeoutSeconds
	}

	return config, nil
}

// composeProposal reads what the turn settled on, if anything new, and checks it.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) composeProposal(session *ComposeSession, raw string, sink runlog.Sink) *ComposeProposal {
	proposal, err := compose.ReadProposal(session.dir)
	if err != nil {
		runlog.Warnf(sink, "读取本轮提案失败：%v", err)

		return nil
	}

	if proposal == nil {
		proposal = fencedProposal(raw)
		if proposal == nil {
			return nil
		}

		runlog.Warnf(sink, "本轮没有调用 propose_config，改用回复末尾的配置块，仍然会复核")
	}

	encoded, err := json.Marshal(proposal)
	if err != nil {
		return nil
	}

	// the same configuration as last turn means the document simply outlived the
	// turn that wrote it; announcing it again would put a second identical card
	// in the chat.
	if !session.reported(string(encoded)) {
		return nil
	}

	return s.checkComposeProposal(proposal, sink)
}

// checkComposeProposal is informer's own verdict on a proposal.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *Service) checkComposeProposal(proposal *compose.Proposal, sink runlog.Sink) *ComposeProposal {
	draft := &feed.Source{}
	candidate := proposal.Changes.Apply(draft)

	result := &ComposeProposal{
		Title:        proposal.Title,
		Reason:       proposal.Reason,
		ParseType:    candidate.ResolveParseType(),
		Changes:      proposal.Changes,
		Fields:       proposal.Changes.Diff(draft),
		Verification: nil,
		Savable:      false,
	}

	// an agent candidate is not verified here. Doing so would start a second
	// agent process while this conversation's own is still winding down, and it
	// would do it while the process wide agent gate is held - which is the
	// invariant this would be breaking from the inside. It is also weak
	// evidence: an agent run that found eight articles proves far less than a
	// regex that matched eight.
	if result.ParseType == feed.ParseTypeAgent {
		runlog.Warnf(sink, "agent 类型的配置无法自动复核，保存后请用「测试抓取」验证一次")

		result.Verification = &ParseVerification{ //nolint:exhaustruct //nothing ran.
			Ran:  false,
			Note: "agent 类型无法自动复核（会在 agent 内再启动一个 agent），请保存后用「测试抓取」验证一次。",
		}
		result.Savable = ValidateSource(candidate) == nil

		return result
	}

	result.Verification = s.verifyCandidate(candidate, sink)
	result.Savable = result.Verification.Error == "" && result.Verification.ArticleCount > 0

	return result
}

// fencedProposal recovers a proposal from a fenced json block at the end of a
// reply.
//
// It is a fallback, not the contract: propose_config is. A model that described
// its configuration in a block instead of handing it over would otherwise leave
// the user with a conversation that reached an answer and no way to save it, and
// the answer is checked by the same verification either way.
func fencedProposal(raw string) *compose.Proposal {
	block := parsecfg.LastFencedJSON(raw)
	if block == "" {
		return nil
	}

	var proposal compose.Proposal

	err := json.Unmarshal([]byte(block), &proposal)
	if err != nil {
		return nil
	}

	if strings.TrimSpace(proposal.Title) == "" || proposal.Changes.IsEmpty() {
		return nil
	}

	return &proposal
}

// arm records the cancel function of the turn now running.
func (c *ComposeSession) arm(cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cancel = cancel
}

// touch restarts the idle countdown after a turn.
func (c *ComposeSession) touch() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.idle != nil {
		c.idle.Reset(composeIdleTimeout)
	}
}

// reported records the proposal just handed to the caller, reporting whether it
// is a different one from the last.
func (c *ComposeSession) reported(encoded string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if encoded == c.seen {
		return false
	}

	c.seen = encoded

	return true
}

// discardComposeSession stops a conversation and removes what it left on disk.
func discardComposeSession(session *ComposeSession) {
	if session == nil {
		return
	}

	session.mu.Lock()

	if session.idle != nil {
		session.idle.Stop()
	}

	cancel := session.cancel
	session.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	_ = os.RemoveAll(session.dir)
}
