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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
)

// A composing turn and a diagnosis share one gate, so a conversation cannot
// start a second agent command line while a diagnosis is still driving one.
func TestComposeChatRefusesWhileAnotherAgentRunIsInFlight(t *testing.T) {
	t.Parallel()

	app := noAgentApp(t)

	app.agentMu.Lock()
	defer app.agentMu.Unlock()

	_, err := app.ComposeChat("any-session", "hello", "")
	require.ErrorIs(t, err, ErrComposeRunning)

	// the same gate answers the other feature, which is the point of sharing it.
	_, err = app.DiagnoseSource(1, "")
	require.ErrorIs(t, err, ErrDiagnoseRunning)
}

// Every composing binding refuses to touch data when startup failed, exactly
// like the rest of the desktop contract.
func TestComposeBindingsRefuseABrokenStartup(t *testing.T) {
	t.Parallel()

	broken := &App{initErr: assert.AnError} //nolint:exhaustruct //a failed startup is the fixture.

	_, err := broken.StartCompose()
	require.ErrorIs(t, err, ErrNotReady)

	_, err = broken.ComposeChat("s", "hello", "")
	require.ErrorIs(t, err, ErrNotReady)

	require.ErrorIs(t, broken.CloseCompose("s"), ErrNotReady)

	_, err = broken.CreateSourceFromCompose(`{"url":"https://example.com"}`, "blog", 0)
	require.ErrorIs(t, err, ErrNotReady)
}

// Saving is the only write of the whole composing path, and it refuses anything
// that is not a configuration the page got from a proposal.
func TestCreateSourceFromComposeRefusesWhatIsNotAProposal(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	_, err := app.CreateSourceFromCompose("", "blog", 0)
	require.ErrorIs(t, err, ErrNoConfigToSave)

	_, err = app.CreateSourceFromCompose("not json", "blog", 0)
	require.ErrorIs(t, err, ErrNoConfigToSave)
}

// The opaque configuration travels out with a proposal and back in unchanged;
// what comes out the other end is a stored subscription.
func TestCreateSourceFromComposeStoresTheProposal(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	created, err := app.CreateSourceFromCompose(
		`{"url":"https://example.com/atom.xml","parse_type":"feed"}`, "Example blog", 0)
	require.NoError(t, err)

	require.Equal(t, "Example blog", created.Title)
	require.Equal(t, "https://example.com/atom.xml", created.URL)
	require.Equal(t, feed.ParseTypeFeed, created.ParseType)
	require.True(t, created.Enabled)

	stored := storedSource(t, app, created.ID)
	require.NotNil(t, stored)
	require.Equal(t, "Example blog", stored.Title)

	// a configuration with neither an address nor an agent prompt is refused by
	// the same validation every created source goes through.
	_, err = app.CreateSourceFromCompose(`{"parse_type":"regex","regex":"x"}`, "Nowhere", 0)
	require.Error(t, err)
}

// Starting a conversation runs no agent: it only opens the run directory the
// turns will share, so it must work on a machine with no agent installed.
func TestStartComposeRunsNoAgent(t *testing.T) {
	t.Parallel()

	app := noAgentApp(t)

	id, err := app.StartCompose()
	require.NoError(t, err)
	require.NotEmpty(t, id)

	require.NoError(t, app.CloseCompose(id))
}
