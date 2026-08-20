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
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/compose"
	"github.com/vogo/informer/internal/diagnose"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/runlog"
	"github.com/vogo/informer/internal/service"
)

// agentE2EEnv opts into the one test that drives a real agent command line.
//
// It is off by default because that run takes minutes and spends real api
// budget, which no CI job and no `go test ./...` should do by surprise. It is
// the only test that proves the parts actually connect - the mcp server the
// agent reaches is this very test binary, launched through TestMain below - so
// it is kept runnable rather than deleted:
//
//	INFORMER_AGENT_E2E=1 go test ./internal/service -run DiagnoseSourceEndToEnd -v
const agentE2EEnv = "INFORMER_AGENT_E2E"

// TestMain doubles this test binary as the tool server of both agent features.
//
// A diagnosis and a composing conversation each launch os.Executable() to reach
// their tools, and under `go test` that executable is this binary. Answering
// both sub commands here is what lets the end to end tests exercise the real
// wiring instead of a stand in. There is one TestMain per package, so this is
// the only place either can be answered.
func TestMain(m *testing.M) {
	if dir, ok := diagnose.ServeArgs(os.Args[1:]); ok {
		os.Exit(serveExitCode(diagnose.ServeStdio(context.Background(), dir, "test")))
	}

	if dir, ok := compose.ServeArgs(os.Args[1:]); ok {
		os.Exit(serveExitCode(compose.ServeStdio(context.Background(), dir, "test")))
	}

	os.Exit(m.Run())
}

// serveExitCode turns a tool server outcome into a process exit code.
func serveExitCode(err error) int {
	if err != nil {
		return 1
	}

	return 0
}

// TestDiagnoseSourceEndToEnd drives the whole feature against a real agent: a
// page whose markup changed, a stored regex that no longer matches, and a run
// that has to read the page, try candidates and come back with one that parses.
//
// What is asserted is deliberately not "the agent found this exact regex" - that
// is not reproducible - but the contract around it: the run concludes, the
// source is untouched, and a proposal is offered only when informer's own
// re-parse produced articles.
//
//nolint:gosmopolitan //informer is a chinese product; the fixtures speak the user's language.
func TestDiagnoseSourceEndToEnd(t *testing.T) {
	if os.Getenv(agentE2EEnv) == "" {
		t.Skipf("set %s=1 to run the real agent end to end", agentE2EEnv)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
			<nav><a href="/about">关于</a><a href="/rss">订阅</a></nav>
			<ul class="post-list">
			<li class="post-item"><a class="post-link" href="/p/1001">Go 1.25 泛型改进实践</a></li>
			<li class="post-item"><a class="post-link" href="/p/1002">用 eBPF 定位线上抖动</a></li>
			<li class="post-item"><a class="post-link" href="/p/1003">SQLite 并发写入的真相</a></li>
			</ul>
			<footer><a href="/contact">联系我们</a></footer>
			</body></html>`))
	}))
	defer server.Close()

	svc := newService(t)

	source := &feed.Source{
		Title:     "某技术博客",
		URL:       server.URL,
		ParseType: feed.ParseTypeRegex,
		// the markup this regex was written for is gone from the page above.
		Regex:     `<li class="item">\s*<a href="([^"]+)">([^<]+)</a>`,
		URLExp:    urlExp,
		TitleExp:  titleExp,
		ErrorInfo: noMatch,
	}
	require.NoError(t, svc.CreateSource(source))

	before, err := svc.GetSource(source.ID)
	require.NoError(t, err)

	sink := runlog.FuncSink(func(entry runlog.Entry) {
		t.Logf("[%s] %s", entry.Level, entry.Text)
	})

	report, err := svc.DiagnoseSource(t.Context(), source.ID, sink)
	require.NoError(t, err)

	t.Logf("diagnosis: %s", report.Diagnosis)
	t.Logf("advice: %s", report.Advice)

	after, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "a diagnosis never writes")

	require.NotEmpty(t, report.Diagnosis, "a run always explains itself")

	if !report.Fixed {
		require.NotEmpty(t, report.Advice, "a diagnosis that repairs nothing has to say what to do instead")

		return
	}

	requireVerifiedFix(t, report)
	requireAppliedFix(t, svc, source.ID, report, sink)
}

// requireVerifiedFix asserts the contract behind a report that offers a fix:
// informer's own verdict is the one the apply button follows, so it has to be
// backed by informer's own parse rather than by the agent's word.
func requireVerifiedFix(t *testing.T, report *service.DiagnoseReport) {
	t.Helper()

	require.NotNil(t, report.Changes)
	require.NotEmpty(t, report.Diff)
	require.True(t, report.Verification.Ran)
	require.Empty(t, report.Verification.Error)
	require.Positive(t, report.Verification.ArticleCount)

	t.Logf("verified %d articles, first: %s", report.Verification.ArticleCount,
		report.Verification.Samples[0].Title)
}

// requireAppliedFix asserts what pressing apply does: the change lands, the
// source is healthy again, and it parses the three posts - not the nav bar.
func requireAppliedFix(t *testing.T, svc *service.Service, sourceID int64,
	report *service.DiagnoseReport, sink runlog.Sink,
) {
	t.Helper()

	require.NoError(t, svc.ApplySourceFix(sourceID, report.Changes, sink))

	repaired, err := svc.GetSource(sourceID)
	require.NoError(t, err)
	require.Equal(t, feed.StatusNormal, repaired.Status)

	articles, err := svc.Preview(sourceID)
	require.NoError(t, err)
	require.Len(t, articles, 3, "the three posts, and not the nav or footer links")
}
