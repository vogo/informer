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

// This file tests from inside the package on purpose.
//
// buildDiagnoseReport and verifyDiagnose are unexported, and they are where the
// whole diagnosis feature's safety net lives: they decide whether informer
// offers a one click fix at all. Reaching them through DiagnoseSource would mean
// driving a real agent, which no ordinary test run may do, so they are exercised
// here directly with a report the agent might have returned.

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/diagnose"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/runlog"
)

// Regexes the cases below share: the one the page stopped answering to, and the
// one that reads it as it is now.
const (
	staleRegex  = `<a class="entry" href="([^"]+)">([^<]+)</a>`
	freshRegex  = `<a class="post" href="([^"]+)">([^<]+)</a>`
	uselessLink = "$1"
	uselessName = "$2"
)

// internalService opens a service whose agent command line does not exist, so
// nothing here can start an agent by accident and the binary lookup fails at
// once instead of probing a login shell.
func internalService(t *testing.T) *Service {
	t.Helper()

	svc, err := New(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, svc.SaveAgentConfig(&agent.Config{
		Provider: agent.ProviderClaude,
		Command:  filepath.Join(t.TempDir(), "no-such-agent"),
	}))

	return svc
}

// postsPage serves however many posts are asked for, in markup freshRegex reads
// and staleRegex does not.
func postsPage(t *testing.T, posts int) *httptest.Server {
	t.Helper()

	var body strings.Builder

	body.WriteString("<ul>")

	for i := 1; i <= posts; i++ {
		body.WriteString(`<li><a class="post" href="/p/`)
		body.WriteRune(rune('0' + i%10))
		body.WriteString(`.html">Post `)
		body.WriteRune(rune('0' + i%10))
		body.WriteString(`</a></li>`)
	}

	body.WriteString("</ul>")

	page := body.String()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))

	t.Cleanup(server.Close)

	return server
}

// staleSource is a stored source whose regex the page no longer answers to.
func staleSource(server *httptest.Server) *feed.Source {
	return &feed.Source{
		ID:        7,
		Title:     "a blog",
		URL:       server.URL,
		ParseType: feed.ParseTypeRegex,
		Regex:     staleRegex,
		URLExp:    uselessLink,
		TitleExp:  uselessName,
	}
}

func TestVerifyDiagnoseAcceptsAProposalThatParses(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 2))

	verification := svc.verifyDiagnose(
		(&diagnose.Changes{Regex: strPtr(freshRegex)}).Apply(source), nil)

	require.True(t, verification.Ran)
	require.Empty(t, verification.Error)
	require.Equal(t, 2, verification.ArticleCount)
	require.Len(t, verification.Samples, 2)
	require.Contains(t, verification.Samples[0].Title, "Post")
}

func TestVerifyDiagnoseRejectsAProposalThatStillFails(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 2))

	verification := svc.verifyDiagnose(
		(&diagnose.Changes{Regex: strPtr(`<a class="nope" href="([^"]+)">([^<]+)</a>`)}).Apply(source), nil)

	require.True(t, verification.Ran)
	require.NotEmpty(t, verification.Error)
	require.Zero(t, verification.ArticleCount)
}

// A proposal that leaves the source unusable never reaches the network: it is
// refused by validation, which is the cheaper and clearer failure.
func TestVerifyDiagnoseRefusesAnInvalidCandidate(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 2))

	verification := svc.verifyDiagnose(
		(&diagnose.Changes{URL: strPtr(""), CURL: strPtr("")}).Apply(source), nil)

	require.True(t, verification.Ran)
	require.Contains(t, verification.Error, "neither url nor curl")
	require.Zero(t, verification.ArticleCount)
}

// The report quotes only the first handful of articles: a person is checking
// that the titles are articles at all, not reading the whole feed.
func TestVerifyDiagnoseQuotesOnlyASampleOfALongResult(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, diagnoseSampleArticles+5))

	verification := svc.verifyDiagnose(
		(&diagnose.Changes{Regex: strPtr(freshRegex)}).Apply(source), nil)

	require.Empty(t, verification.Error)
	require.Equal(t, diagnoseSampleArticles+5, verification.ArticleCount)
	require.Len(t, verification.Samples, diagnoseSampleArticles)
}

func TestBuildDiagnoseReportOffersAVerifiedFix(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 3))

	result := svc.buildDiagnoseReport(source, &diagnose.Report{
		Fixed:     true,
		Diagnosis: "the markup changed",
		Changes:   &diagnose.Changes{Regex: strPtr(freshRegex)},
	}, nil)

	require.True(t, result.Fixed)
	require.True(t, result.AgentClaimedFixed)
	require.Equal(t, source.ID, result.SourceID)
	require.Equal(t, "the markup changed", result.Diagnosis)

	require.NotNil(t, result.Changes)
	require.Len(t, result.Diff, 1)
	require.Equal(t, "regex", result.Diff[0].Field)
	require.Equal(t, staleRegex, result.Diff[0].Old)
	require.Equal(t, freshRegex, result.Diff[0].New)

	require.Equal(t, 3, result.Verification.ArticleCount)
}

// The case the whole design exists for: the agent says it fixed the source and
// informer's own re-parse disagrees. The agent's claim is recorded, the verdict
// is not, and no fix is offered.
//
//nolint:gosmopolitan //asserts on the chinese line informer records.
func TestBuildDiagnoseReportRefusesAFixItsOwnRecheckFails(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 3))

	var problems []string

	sink := runlog.FuncSink(func(entry runlog.Entry) {
		if entry.Level != runlog.LevelInfo {
			problems = append(problems, entry.Text)
		}
	})

	result := svc.buildDiagnoseReport(source, &diagnose.Report{
		Fixed:     true,
		Diagnosis: "I fixed it",
		Changes:   &diagnose.Changes{Regex: strPtr(`<a class="nope" href="([^"]+)">([^<]+)</a>`)},
	}, sink)

	require.False(t, result.Fixed, "informer's own recheck decides")
	require.True(t, result.AgentClaimedFixed, "and the agent's claim is kept, not hidden")
	require.NotEmpty(t, result.Verification.Error)

	require.Contains(t, strings.Join(problems, "\n"), "复核没有通过")
}

// emptyAtom is a well formed feed that lists nothing, which is how a parse
// succeeds and still produces no articles.
const emptyAtom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Example</title></feed>`

// A proposal that parses without error but yields nothing is the usual
// half-fixed state, and it must not be offered as a fix either: switching this
// source to the site's new feed "works", and subscribes the user to silence.
//
//nolint:gosmopolitan //asserts on the chinese line informer records.
func TestBuildDiagnoseReportRefusesAFixThatParsesNothing(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 3))

	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(emptyAtom))
	}))
	defer feedServer.Close()

	var problems []string

	sink := runlog.FuncSink(func(entry runlog.Entry) {
		if entry.Level != runlog.LevelInfo {
			problems = append(problems, entry.Text)
		}
	})

	result := svc.buildDiagnoseReport(source, &diagnose.Report{
		Fixed:     true,
		Diagnosis: "the site publishes a feed now",
		Changes: &diagnose.Changes{
			ParseType: strPtr(feed.ParseTypeFeed),
			URL:       strPtr(feedServer.URL),
		},
	}, sink)

	require.False(t, result.Fixed)
	require.Empty(t, result.Verification.Error, "parsing succeeded; it simply found nothing")
	require.Zero(t, result.Verification.ArticleCount)
	require.Len(t, result.Diff, 2, "both the type and the address are proposed changes")

	require.Contains(t, strings.Join(problems, "\n"), "一条都没有取到")
}

// "I could not fix this" is a legitimate outcome: the advice survives and no
// verification is attempted, because there is nothing to verify.
func TestBuildDiagnoseReportKeepsAnUnfixableAnswer(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 3))

	result := svc.buildDiagnoseReport(source, &diagnose.Report{
		Diagnosis: "the site is gone",
		Advice:    "delete this subscription",
	}, nil)

	require.False(t, result.Fixed)
	require.Nil(t, result.Changes)
	require.Empty(t, result.Diff)
	require.Equal(t, "delete this subscription", result.Advice)
	require.False(t, result.Verification.Ran, "nothing was proposed, so nothing was re-parsed")
}

// An agent that echoes the stored configuration back has proposed nothing, and
// must not be presented as having found a fix.
//
//nolint:gosmopolitan //asserts on the chinese line informer records.
func TestBuildDiagnoseReportTreatsAnEchoAsNoProposal(t *testing.T) {
	svc := internalService(t)
	source := staleSource(postsPage(t, 3))

	var problems []string

	sink := runlog.FuncSink(func(entry runlog.Entry) {
		if entry.Level != runlog.LevelInfo {
			problems = append(problems, entry.Text)
		}
	})

	result := svc.buildDiagnoseReport(source, &diagnose.Report{
		Fixed:     true,
		Diagnosis: "looks fine to me",
		Changes:   &diagnose.Changes{Regex: strPtr(staleRegex), TitleExp: strPtr(uselessName)},
	}, sink)

	require.False(t, result.Fixed)
	require.Nil(t, result.Changes)
	require.False(t, result.Verification.Ran)
	require.Contains(t, strings.Join(problems, "\n"), "没有给出可用的配置改动")
}

// diagnoseObserver is allowed to be handed no sink at all, which is what an
// unwatched run passes.
func TestDiagnoseObserverToleratesNoSink(t *testing.T) {
	require.Nil(t, diagnoseObserver(nil))

	var seen []string

	observer := diagnoseObserver(runlog.FuncSink(func(entry runlog.Entry) {
		seen = append(seen, entry.Level+":"+entry.Text)
	}))
	require.NotNil(t, observer)

	observer.Note(runlog.LevelWarn, "  searching  ")

	require.Equal(t, []string{"warn:searching"}, seen, "the note is trimmed before it is recorded")
}

// strPtr is the pointer-taking helper the Changes shape needs; a nil field and a
// field set to empty are different answers, so every value travels as a pointer.
func strPtr(v string) *string { return &v }
