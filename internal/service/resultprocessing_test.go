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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/resultprocess"
	"github.com/vogo/informer/internal/service"
)

const (
	finalPrompt    = "keep only the most useful article"
	filteredAnswer = `{"action":"publish","summary":"daily digest","articles":[{"id":2,"reason":"high value"}],"rejected":[{"id":1,"reason":"low value"}]}`
)

// processingService seeds two candidates without relying on external feeds.
func processingService(t *testing.T, answer string) *service.Service {
	t.Helper()
	svc := newService(t)
	stubAgent(t, svc, answer)
	require.NoError(t, svc.CreateArticle(&feed.Article{Title: "low value", URL: "https://example.com/one", Timestamp: time.Now().Unix(), Weight: 10}))
	require.NoError(t, svc.CreateArticle(&feed.Article{Title: "high value", URL: "https://example.com/two", Timestamp: time.Now().Unix(), Weight: 20}))
	require.NoError(t, svc.SaveResultProcessingConfig(&resultprocess.Config{Enabled: true, Prompt: finalPrompt}))
	// Keep soup local and instantaneous for every full-run test.
	server := newContentServer(t)
	freezeDailySoup(t, server.URL+"/broken")

	return svc
}

func TestFinalProcessingUsesOnlySelectedArticles(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	result, err := svc.TriggerInform("")
	require.NoError(t, err)
	require.Len(t, result.Articles, 1)
	assert.Equal(t, int64(2), result.Articles[0].ID)
	assert.Contains(t, result.Content, "daily digest")
	assert.Contains(t, result.Content, "https://example.com/two")
	assert.NotContains(t, result.Content, "https://example.com/one")
	require.FileExists(t, result.ContentFilePath)
	require.FileExists(t, result.ProcessingRecordPath)
	articles := allArticles(t, svc)
	for _, article := range articles {
		assert.Equal(t, article.ID == 2, article.Informed)
		assert.Equal(t, article.ID == 2, article.InformedAt != nil)
	}
	// Rebuilding from the final Markdown cannot credit a filtered candidate.
	rebuilt, err := svc.RebuildHistoryIndex()
	require.NoError(t, err)
	assert.Equal(t, 1, rebuilt.Links)
	assert.Zero(t, rebuilt.Filled)
}

func TestFinalProcessingDeliversFinalContent(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		received <- string(raw)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	t.Cleanup(server.Close)
	result, err := svc.TriggerInform(server.URL + "/feishu.cn/ok")
	require.NoError(t, err)
	assert.True(t, result.Notified)
	assert.Len(t, result.Articles, 1)
	payload := <-received
	assert.Contains(t, payload, "daily digest")
	assert.Contains(t, payload, "https://example.com/two")
	assert.NotContains(t, payload, "https://example.com/one")
}

func TestFinalProcessingSkipConsumesNothing(t *testing.T) {
	svc := processingService(t, `{"action":"skip","summary":"nothing qualifies","articles":[],"rejected":[{"id":1,"reason":"skip"},{"id":2,"reason":"skip"}]}`)
	result, err := svc.TriggerInform("https://unsupported.example")
	require.NoError(t, err)
	assert.True(t, result.Skipped)
	assert.Equal(t, "nothing qualifies", result.SkipReason)
	assert.Empty(t, result.Deliveries)
	assert.Empty(t, result.ContentFilePath)
	for _, article := range allArticles(t, svc) {
		assert.False(t, article.Informed)
		assert.Nil(t, article.InformedAt)
	}
	index, err := svc.DailyIndex()
	require.NoError(t, err)
	assert.Empty(t, index)
}

func TestFinalProcessingFailureKeepsCandidatesAndPriorReport(t *testing.T) {
	for _, answer := range []string{"not json", `{"action":"publish","summary":"bad","articles":[{"id":999,"reason":"invented"}]}`} {
		t.Run(answer, func(t *testing.T) {
			svc := processingService(t, answer)
			path := inform.DailyFilePath(svc.HomeDir(), time.Now())
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte("prior report"), 0o600))
			result, err := svc.TriggerInform("https://unsupported.example")
			require.ErrorIs(t, err, resultprocess.ErrInvalidResult)
			assert.Empty(t, result.Deliveries)
			assert.Empty(t, result.ContentFilePath)
			assert.FileExists(t, result.ProcessingRecordPath)
			prior, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "prior report", string(prior))
			audit, err := os.ReadFile(result.ProcessingRecordPath)
			require.NoError(t, err)
			assert.Contains(t, string(audit), "https://example.com/one")
			assert.Contains(t, string(audit), "error")
			for _, article := range allArticles(t, svc) {
				assert.False(t, article.Informed)
				assert.Nil(t, article.InformedAt)
			}
		})
	}
}

func TestFinalProcessingDeliveryFailureLeavesBothFlagsUnset(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	server := newContentServer(t)
	result, err := svc.TriggerInform(server.URL + "/feishu.cn/fail")
	require.Error(t, err)
	assert.FileExists(t, result.ContentFilePath)
	for _, article := range allArticles(t, svc) {
		assert.False(t, article.Informed)
		assert.Nil(t, article.InformedAt)
	}
	// Retry selects the same candidates instead of losing the filtered/failed batch.
	result, err = svc.TriggerInform(server.URL + "/feishu.cn/ok")
	require.NoError(t, err)
	assert.Len(t, result.Articles, 1)
}

func TestFinalProcessingPreviewDoesNotWrite(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	before := snapshotDB(t, svc)
	configBefore, err := os.ReadFile(svc.ConfigFilePath())
	require.NoError(t, err)
	preview, err := svc.PreviewResultProcessing(context.Background(), finalPrompt)
	require.NoError(t, err)
	assert.Len(t, preview.Candidates, 2)
	assert.Contains(t, preview.Report.Content, "daily digest")
	assert.Equal(t, before, snapshotDB(t, svc))
	configAfter, err := os.ReadFile(svc.ConfigFilePath())
	require.NoError(t, err)
	assert.Equal(t, configBefore, configAfter)
	_, err = os.Stat(filepath.Join(svc.HomeDir(), "runs"))
	assert.True(t, os.IsNotExist(err))
	index, err := svc.DailyIndex()
	require.NoError(t, err)
	assert.Empty(t, index)
}

func TestFinalProcessingConfigValidationAndPreservation(t *testing.T) {
	svc := newService(t)
	require.ErrorIs(t, svc.SaveResultProcessingConfig(nil), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.SaveResultProcessingConfig(&resultprocess.Config{Enabled: true, Prompt: "  "}), service.ErrInvalidArgument)
	require.NoError(t, svc.SaveResultProcessingConfig(&resultprocess.Config{Prompt: finalPrompt}))
	require.NoError(t, svc.SaveFeedConfig(service.DefaultFeedConfig()))
	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.False(t, view.ResultProcessing.Enabled)
	assert.Equal(t, finalPrompt, view.ResultProcessing.Prompt)
	assert.NotContains(t, view.PreservedKeys, "result_processing")
	require.NoError(t, svc.SaveResultProcessingConfig(&resultprocess.Config{Enabled: true, Prompt: finalPrompt}))
	view, err = svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.True(t, view.ResultProcessing.Enabled)
}

func TestFinalProcessingPreviewCancellation(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.PreviewResultProcessing(ctx, finalPrompt)
	require.ErrorIs(t, err, context.Canceled)
	for _, article := range allArticles(t, svc) {
		assert.False(t, article.Informed)
		assert.Nil(t, article.InformedAt)
	}
}

func TestFinalProcessingAgentFailureKeepsCandidates(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	require.NoError(t, svc.SaveAgentConfig(&agent.Config{Command: filepath.Join(t.TempDir(), "missing-agent")}))
	result, err := svc.TriggerInform("https://unsupported.example")
	require.Error(t, err)
	assert.Empty(t, result.Deliveries)
	assert.Empty(t, result.ContentFilePath)
	assert.FileExists(t, result.ProcessingRecordPath)
	for _, article := range allArticles(t, svc) {
		assert.False(t, article.Informed)
		assert.Nil(t, article.InformedAt)
	}
}

func TestFinalProcessingWriteFailureDoesNotConsumeCandidates(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	// A directory at the final file path makes the atomic rename fail.
	path := inform.DailyFilePath(svc.HomeDir(), time.Now())
	require.NoError(t, os.MkdirAll(path, 0o700))
	result, err := svc.TriggerInform("https://unsupported.example")
	require.ErrorContains(t, err, "write daily report")
	assert.Empty(t, result.Deliveries)
	assert.Empty(t, result.ContentFilePath)
	for _, article := range allArticles(t, svc) {
		assert.False(t, article.Informed)
		assert.Nil(t, article.InformedAt)
	}
}

func TestFinalProcessingDisablesAgentTools(t *testing.T) {
	svc := processingService(t, filteredAnswer)
	promptFile := stubAgent(t, svc, filteredAnswer)
	_, err := svc.PreviewResultProcessing(context.Background(), finalPrompt)
	require.NoError(t, err)
	args, err := os.ReadFile(promptFile)
	require.NoError(t, err)
	assert.Contains(t, string(args), "--tools\n\n")
	assert.NotContains(t, string(args), "--allowedTools")
	assert.Contains(t, string(args), "--strict-mcp-config")
}

func TestFinalProcessingPreviewUsesServiceDatabase(t *testing.T) {
	first := processingService(t, filteredAnswer)
	second := newService(t)
	beforeFirst := snapshotDB(t, first)
	beforeSecond := snapshotDB(t, second)

	preview, err := first.PreviewResultProcessing(context.Background(), finalPrompt)
	require.NoError(t, err)
	require.Len(t, preview.Candidates, 2)
	assert.Equal(t, "low value", preview.Candidates[1].Title)
	assert.Contains(t, preview.Report.Content, "https://example.com/two")
	assert.Equal(t, beforeFirst, snapshotDB(t, first))
	assert.Equal(t, beforeSecond, snapshotDB(t, second))
}
