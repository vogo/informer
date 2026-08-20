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
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// brokenListing serves a page whose markup the stored regex below no longer
// matches, which is the failure the diagnosis feature exists for.
//
//nolint:gosmopolitan //the fixtures quote the shipped chinese text.
func brokenListing(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<li><a class="post" href="/a.html">第一篇文章</a></li>`))
	}))

	t.Cleanup(server.Close)

	return server
}

// noAgentApp is a test app pointed at an agent command line that does not
// exist, so a diagnosis fails the moment it tries to start one.
//
// A test must never spend minutes - and a real api budget - driving whatever
// agent happens to be installed on the machine it runs on.
func noAgentApp(t *testing.T) *App {
	t.Helper()

	app := newTestApp(t)

	require.NoError(t, app.SaveAgentConfig(&AgentConfigDTO{
		Provider: agent.ProviderClaude,
		Command:  filepath.Join(t.TempDir(), "no-such-agent"),
	}))

	return app
}

// storedSource reads one subscription back through the listing, which is the
// only read the desktop contract offers.
func storedSource(t *testing.T, app *App, id int64) *SourceDTO {
	t.Helper()

	listed, err := app.ListSources(nil)
	require.NoError(t, err)

	for _, source := range listed {
		if source.ID == id {
			return source
		}
	}

	t.Fatalf("source %d is gone", id)

	return nil
}

// brokenRequest is a subscription whose regex stopped matching brokenListing.
//
//nolint:gosmopolitan //the fixtures quote the shipped chinese text.
func brokenRequest(server *httptest.Server) *SaveSourceRequest {
	return &SaveSourceRequest{
		Title:     "测试站",
		URL:       server.URL,
		ParseType: feed.ParseTypeRegex,
		Regex:     `<a class="entry" href="([^"]+)">([^<]+)</a>`,
		URLExp:    "$1",
		TitleExp:  "$2",
	}
}

// The whole point of the opaque fix payload: the page hands back what it was
// given, and the binding turns it into a stored configuration.
func TestApplySourceFixStoresTheProposal(t *testing.T) {
	t.Parallel()

	server := brokenListing(t)
	app := newTestApp(t)

	created, err := app.CreateSource(brokenRequest(server))
	require.NoError(t, err)

	err = app.ApplySourceFix(created.ID,
		`{"regex":"<a class=\"post\" href=\"([^\"]+)\">([^<]+)</a>"}`, "")
	require.NoError(t, err)

	stored := storedSource(t, app, created.ID)
	assert.Equal(t, `<a class="post" href="([^"]+)">([^<]+)</a>`, stored.Regex)
	assert.Equal(t, "$2", stored.TitleExp, "an untouched field survives the repair")
	assert.Equal(t, feed.StatusNormal, stored.Status, "a source that parses again is healthy again")

	articles, err := app.PreviewSource(created.ID, "")
	require.NoError(t, err)
	assert.Len(t, articles, 1)
}

// An empty or unreadable payload has to be refused rather than written as a
// no-op: the only call of this path that writes must never write a guess.
func TestApplySourceFixRefusesAnUnusablePayload(t *testing.T) {
	t.Parallel()

	server := brokenListing(t)
	app := newTestApp(t)

	created, err := app.CreateSource(brokenRequest(server))
	require.NoError(t, err)

	require.ErrorIs(t, app.ApplySourceFix(created.ID, "", ""), ErrNoFixToApply)
	require.ErrorIs(t, app.ApplySourceFix(created.ID, "not json", ""), ErrNoFixToApply)

	// a well formed payload that proposes nothing is refused by the service.
	require.ErrorIs(t, app.ApplySourceFix(created.ID, "{}", ""), service.ErrInvalidArgument)

	stored := storedSource(t, app, created.ID)
	assert.Equal(t, `<a class="entry" href="([^"]+)">([^<]+)</a>`, stored.Regex,
		"a refused apply changes nothing")
}

// A diagnosis writes nothing, whatever happens to the agent run. The run fails
// here because no agent is configured, which is exactly when the source has to
// come through untouched.
func TestDiagnoseSourceNeverWritesTheSource(t *testing.T) {
	t.Parallel()

	server := brokenListing(t)
	app := noAgentApp(t)

	created, err := app.CreateSource(brokenRequest(server))
	require.NoError(t, err)

	report, err := app.DiagnoseSource(created.ID, "run-1")
	require.Error(t, err, "no agent command line exists in a test process")
	require.Nil(t, report, "a run that could not conclude reports nothing")

	assert.Equal(t, created, storedSource(t, app, created.ID))
}

// TestDiagnoseSourceWithARunIDNeedsNoWindow is the regression test of the nil
// application guard: a binding test runs in a process that never called
// application.New, and a diagnosis asking for progress must not crash there.
func TestDiagnoseSourceWithARunIDNeedsNoWindow(t *testing.T) {
	t.Parallel()

	app := noAgentApp(t)

	require.NotPanics(t, func() {
		_, _ = app.DiagnoseSource(404, "run-1")
	})
}

func TestDiagnoseSinkForSkipsAnEmptyRunID(t *testing.T) {
	t.Parallel()

	assert.Nil(t, diagnoseSinkFor(""))
	assert.NotNil(t, diagnoseSinkFor("run-1"))
}

// The two watched runs must not report on the same channel, or a diagnosis
// started from a card would scroll its lines into an open test fetch panel.
func TestDiagnoseAndPreviewReportOnDifferentEvents(t *testing.T) {
	t.Parallel()

	assert.NotEqual(t, PreviewLogEvent, DiagnoseLogEvent)
}

// toVerificationDTO is reached only after a real agent run, so it is exercised
// here directly: the samples the page shows above the apply button come through
// it, and an absent verification must not become a null the page dereferences.
func TestVerificationDTOAlwaysCarriesASampleList(t *testing.T) {
	t.Parallel()

	absent := toVerificationDTO(nil)
	require.NotNil(t, absent)
	assert.False(t, absent.Ran)
	assert.NotNil(t, absent.Samples, "the page iterates this without a nil check")
	assert.Empty(t, absent.Samples)

	empty := toVerificationDTO(&service.DiagnoseVerification{Ran: true, Error: "no match"})
	assert.True(t, empty.Ran)
	assert.Equal(t, "no match", empty.Error)
	assert.NotNil(t, empty.Samples)
	assert.Empty(t, empty.Samples)

	filled := toVerificationDTO(&service.DiagnoseVerification{
		Ran:          true,
		ArticleCount: 42,
		Samples: []*feed.Article{
			{Title: "first post", URL: "https://example.com/1"},
			{Title: "second post", URL: "https://example.com/2"},
		},
	})
	assert.Equal(t, 42, filled.ArticleCount, "the count is the whole result, not the sample size")
	require.Len(t, filled.Samples, 2)
	assert.Equal(t, "first post", filled.Samples[0].Title)
	assert.Equal(t, "https://example.com/2", filled.Samples[1].URL)
}

// Two diagnoses at once would drive two agent command lines and spend twice the
// budget, which is far more often a double click than an intention.
func TestDiagnoseSourceRefusesASecondConcurrentRun(t *testing.T) {
	t.Parallel()

	app := noAgentApp(t)

	app.diagnoseMu.Lock()
	defer app.diagnoseMu.Unlock()

	_, err := app.DiagnoseSource(1, "")
	require.ErrorIs(t, err, ErrDiagnoseRunning)
}

// Every diagnosis binding refuses to touch data when startup failed, exactly
// like the rest of the desktop contract.
func TestDiagnoseBindingsRefuseABrokenStartup(t *testing.T) {
	t.Parallel()

	broken := &App{initErr: assert.AnError}

	_, err := broken.DiagnoseSource(1, "")
	require.ErrorIs(t, err, ErrNotReady)

	require.ErrorIs(t, broken.ApplySourceFix(1, `{"regex":"x"}`, ""), ErrNotReady)
}
