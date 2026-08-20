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

package agent

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
)

// Session is a conversation with the command line agent that outlives one run.
//
// Every turn is still its own process: the command line is asked to name a
// conversation on the first turn and to resume it on each later one, and the
// transcript it keeps is what carries the earlier turns forward. That is the
// whole trick, and it has one consequence worth stating plainly - a turn resends
// the conversation so far, so a long conversation costs more than a short one at
// both ends.
//
// A session is not a session id with a struct around it. It also decides what
// happens when a turn dies: the id it claimed cannot be claimed again, and the
// transcript it may have half written is not something to continue from, so the
// next turn starts a new conversation rather than pretending nothing happened.
type Session struct {
	// cfg is the configuration every turn runs with, minus the session fields
	// this type owns. It is a copy, so a caller that keeps editing its own
	// configuration does not change a conversation already under way.
	cfg Config

	mu      sync.Mutex
	id      string
	resumed bool
	turns   int
}

// NewSession starts a conversation. No process runs until the first Send.
func NewSession(cfg *Config) *Session {
	session := &Session{} //nolint:exhaustruct //the zero conversation is the point.
	if cfg != nil {
		session.cfg = *cfg
	}

	return session
}

// Send hands one message to the agent and returns its answer.
//
// The first call names a new conversation; every later one continues it. Calls
// are serialized: two turns of one conversation running at once would have the
// command line resume a transcript that is still being written.
func (s *Session) Send(ctx context.Context, prompt string, observer Observer) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.id == "" {
		id, err := newSessionID()
		if err != nil {
			return "", err
		}

		s.id = id
		s.resumed = false
	}

	config := s.cfg
	config.SessionID = s.id
	config.ResumeSession = s.resumed

	answer, err := RunRaw(ctx, &config, prompt, observer)
	if err != nil {
		// the id is spent whether or not the run got anywhere, and a transcript
		// that was killed mid turn is not one to continue from. The next turn
		// opens a new conversation, which loses the history but stays honest
		// about having lost it.
		s.id = ""
		s.resumed = false

		return "", err
	}

	s.resumed = true
	s.turns++

	return answer, nil
}

// Turns reports how many turns of this conversation have completed.
func (s *Session) Turns() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.turns
}

// ID reports the conversation the next turn would continue, empty before the
// first turn and after one that failed. It is for logs, not for control flow.
func (s *Session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.id
}

// newSessionID mints a version 4 uuid, which is the only shape the command line
// accepts as a session name.
//
// It is hand rolled rather than pulled in: sixteen random bytes and two masked
// nibbles is the whole specification, and a desktop application's dependency
// list is not the place to spend a module on it.
func newSessionID() (string, error) {
	var raw [16]byte

	_, err := rand.Read(raw[:])
	if err != nil {
		return "", fmt.Errorf("generate agent session id: %w", err)
	}

	raw[6] = (raw[6] & 0x0f) | 0x40 // version 4
	raw[8] = (raw[8] & 0x3f) | 0x80 // variant 10

	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}
