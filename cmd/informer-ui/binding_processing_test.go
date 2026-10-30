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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/soup"
)

const (
	processingTestPrompt = "filter"
	processingSkipReason = "nothing qualifies"
)

// TestMain keeps full push tests independent of the public daily sentence API.
// Run the desktop suite with -parallel=1: feed's database handle is process-wide.
func TestMain(m *testing.M) {
	sentence := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"content":"test daily sentence"}`))
	}))
	restore := soup.SetURLForTest(sentence.URL)
	code := m.Run()
	restore()
	sentence.Close()
	os.Exit(code)
}

func TestResultProcessingSettingsRoundTrip(t *testing.T) {
	app := newTestApp(t)
	config, err := app.ReadConfig()
	require.NoError(t, err)
	require.NotNil(t, config.ResultProcessing)
	assert.False(t, config.ResultProcessing.Enabled)
	require.Error(t, app.SaveResultProcessing(nil))
	require.Error(t, app.SaveResultProcessing(&ResultProcessingConfigDTO{Enabled: true}))
	require.NoError(t, app.SaveResultProcessing(&ResultProcessingConfigDTO{Enabled: true, Prompt: processingTestPrompt}))
	require.NoError(t, app.SaveConfig(validFeedDTO()))
	config, err = app.ReadConfig()
	require.NoError(t, err)
	assert.True(t, config.ResultProcessing.Enabled)
	assert.Equal(t, processingTestPrompt, config.ResultProcessing.Prompt)
}

func TestResultProcessingSharesTheAgentGuard(t *testing.T) {
	app := newTestApp(t)
	app.agentMu.Lock()
	_, err := app.PreviewResultProcessing(processingTestPrompt)
	require.ErrorIs(t, err, ErrDiagnoseRunning)
	_, err = app.runInform()
	require.ErrorIs(t, err, ErrDiagnoseRunning)
	app.agentMu.Unlock()
	assert.False(t, app.InformRunning())
}

func TestResultProcessingPreviewAndSkippedPush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub agent is a POSIX script")
	}
	app := newTestApp(t)
	require.NoError(t, app.SaveConfig(validFeedDTO()))
	answer := `{"action":"skip","summary":"nothing qualifies","articles":[],"rejected":[{"id":1,"reason":"low value"}]}`
	encoded, err := json.Marshal(answer)
	require.NoError(t, err)
	binary := filepath.Join(t.TempDir(), "stub-agent")
	script := "#!/bin/sh\ncat <<'ANSWER'\n" + `{"type":"result","subtype":"success","is_error":false,"result":` + string(encoded) + "}\nANSWER\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700)) //nolint:gosec //test executable.
	require.NoError(t, app.svc.SaveAgentConfig(&agent.Config{Command: binary}))
	require.NoError(t, app.svc.CreateArticle(&feed.Article{Title: "candidate", URL: "https://example.com/article", Timestamp: time.Now().Unix()}))
	preview, err := app.PreviewResultProcessing(processingTestPrompt)
	require.NoError(t, err)
	assert.Equal(t, "skip", preview.Action)
	require.Len(t, preview.Decisions, 1)
	assert.False(t, preview.Decisions[0].Selected)
	require.NoError(t, app.SaveResultProcessing(&ResultProcessingConfigDTO{Enabled: true, Prompt: processingTestPrompt}))
	result, err := app.TriggerNow()
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.True(t, result.Skipped)
	assert.Equal(t, processingSkipReason, result.SkipReason)
	assert.Empty(t, result.ContentFilePath)
	assert.FileExists(t, result.ProcessingRecordPath)
	assert.Zero(t, result.ArticleCount)
	assert.False(t, result.Notified)
	// Successful skip uses the same nil-error runner contract as the scheduler.
	_, err = app.svc.PreviewResultProcessing(context.Background(), processingTestPrompt)
	require.NoError(t, err)
}

func TestResultProcessingDTOReportsSkip(t *testing.T) {
	result := toInformResultDTO(&inform.Result{Skipped: true, SkipReason: processingSkipReason}, nil)
	assert.True(t, result.Success)
	assert.True(t, result.Skipped)
	assert.False(t, result.Notified)
}
