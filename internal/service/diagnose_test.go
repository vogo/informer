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

package service_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/diagnose"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/runlog"
	"github.com/vogo/informer/internal/service"
)

// Fixtures shared by the diagnosis cases, including the end to end one.
// titleExp and urlExp are declared once for the whole package in transfer_test.go.
const (
	brokenRegex  = `<a class="entry" href="([^"]+)">([^<]+)</a>`
	workingRegex = `<a class="post" href="([^"]+)">([^<]+)</a>`
	noMatch      = "no match"
)

func strp(v string) *string { return &v }

// listingServer answers with a page whose markup the stored regex of
// brokenRegexSource no longer matches, which is the exact failure this whole
// feature exists for.
func listingServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<li><a class="post" href="/a.html">第一篇文章</a></li>
			<li><a class="post" href="/b.html">第二篇文章</a></li>
		`))
	}))

	t.Cleanup(server.Close)

	return server
}

// brokenRegexSource is a stored source whose regex stopped matching.
func brokenRegexSource(server *httptest.Server) *feed.Source {
	return &feed.Source{
		Title:     "测试站",
		URL:       server.URL,
		ParseType: feed.ParseTypeRegex,
		Regex:     brokenRegex,
		URLExp:    urlExp,
		TitleExp:  titleExp,
		Status:    feed.StatusError,
		ErrorInfo: noMatch,
	}
}

// The working fix: applying it stores the new regex and clears the failure the
// card was showing, without any further user action.
func TestApplySourceFixStoresTheChangeAndClearsTheFailure(t *testing.T) {
	server := listingServer(t)
	svc := newService(t)

	source := brokenRegexSource(server)
	require.NoError(t, svc.CreateSource(source))

	// CreateSource does not carry health state; put the source in the failed
	// state a diagnosis would have found it in.
	require.NoError(t, svc.UpdateSourceColumn(source.ID, "status", feed.StatusError))
	require.NoError(t, svc.UpdateSourceColumn(source.ID, "error_info", noMatch))

	changes := &diagnose.Changes{Regex: strp(workingRegex)}

	require.NoError(t, svc.ApplySourceFix(source.ID, changes, nil))

	stored, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	require.Equal(t, workingRegex, stored.Regex)
	require.Equal(t, titleExp, stored.TitleExp, "an untouched field survives the repair")
	require.Equal(t, feed.StatusNormal, stored.Status, "a source that parses again is healthy again")
	require.Empty(t, stored.ErrorInfo)

	articles, err := svc.Preview(source.ID)
	require.NoError(t, err)
	require.Len(t, articles, 2)
}

// A fix that does not actually work is still saved - the user asked for it - but
// the source is left marked as failing rather than reported as repaired.
func TestApplySourceFixKeepsTheFailureWhenTheFixDoesNotWork(t *testing.T) {
	server := listingServer(t)
	svc := newService(t)

	source := brokenRegexSource(server)
	require.NoError(t, svc.CreateSource(source))

	require.NoError(t, svc.ApplySourceFix(source.ID,
		&diagnose.Changes{Regex: strp(`<a class="still-wrong" href="([^"]+)">([^<]+)</a>`)}, nil))

	stored, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	require.Equal(t, `<a class="still-wrong" href="([^"]+)">([^<]+)</a>`, stored.Regex)
	require.Equal(t, feed.StatusError, stored.Status)
	require.NotEmpty(t, stored.ErrorInfo)
}

func TestApplySourceFixRefusesAnEmptyOrIllegalFix(t *testing.T) {
	server := listingServer(t)
	svc := newService(t)

	source := brokenRegexSource(server)
	require.NoError(t, svc.CreateSource(source))

	err := svc.ApplySourceFix(source.ID, nil, nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	err = svc.ApplySourceFix(source.ID, &diagnose.Changes{}, nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	// a fix that empties the address leaves a source no parser could act on.
	err = svc.ApplySourceFix(source.ID, &diagnose.Changes{URL: strp(""), CURL: strp("")}, nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	stored, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	require.Equal(t, server.URL, stored.URL, "a refused fix changes nothing")
}

func TestApplySourceFixReportsAnUnknownSource(t *testing.T) {
	svc := newService(t)

	err := svc.ApplySourceFix(404, &diagnose.Changes{Regex: strp("x")}, nil)
	require.ErrorIs(t, err, service.ErrNotFound)
}

// A diagnosis writes nothing. The run fails here because no agent is configured
// in the test environment, and that is precisely the moment to assert the source
// came through untouched.
func TestDiagnoseSourceLeavesTheSourceUntouched(t *testing.T) {
	server := listingServer(t)
	svc := newService(t)

	source := brokenRegexSource(server)
	require.NoError(t, svc.CreateSource(source))

	// point the agent at a command line that does not exist, so the run fails
	// the moment it tries to start one. A test must never spend minutes - and a
	// real api budget - driving an agent that happens to be installed on the
	// machine it runs on.
	require.NoError(t, svc.SaveAgentConfig(&agent.Config{
		Provider: agent.ProviderClaude,
		Command:  filepath.Join(t.TempDir(), "no-such-agent"),
	}))

	before, err := svc.GetSource(source.ID)
	require.NoError(t, err)

	var lines []runlog.Entry

	sink := runlog.FuncSink(func(entry runlog.Entry) {
		lines = append(lines, entry)
	})

	_, err = svc.DiagnoseSource(t.Context(), source.ID, sink)
	require.Error(t, err, "no agent to run, so the run cannot conclude")

	after, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "a diagnosis reads and runs, it never writes")

	require.NotEmpty(t, lines)

	var retried bool

	for _, entry := range lines {
		if entry.Text == "先用当前配置重试一次，确认失败是否还在" {
			retried = true
		}
	}

	require.True(t, retried, "the run states what it is doing before it does it")
}

func TestDiagnoseSourceReportsAnUnknownSource(t *testing.T) {
	svc := newService(t)

	_, err := svc.DiagnoseSource(t.Context(), 404, nil)
	require.ErrorIs(t, err, service.ErrNotFound)
}
