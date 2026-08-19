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
)

// sampleAtom is the smallest feed the gofeed parser accepts, enough to prove
// a preview round trip without touching the real network.
const sampleAtom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example</title>
  <entry>
    <title>Hello Preview</title>
    <link href="https://example.com/one"/>
    <id>urn:one</id>
  </entry>
</feed>`

func newTestApp(t *testing.T) *App {
	t.Helper()

	app := newAppWithHome(t.TempDir())
	require.Empty(t, app.StartupError())

	return app
}

func sampleRequest() *SaveSourceRequest {
	return &SaveSourceRequest{
		Title:     "example",
		URL:       "https://example.com/atom.xml",
		ParseType: "feed",
	}
}

func TestSourceCRUD(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	empty, err := app.ListSources(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	created, err := app.CreateSource(sampleRequest())
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.True(t, created.Enabled, "a created source is enabled by the service")
	assert.Equal(t, int64(1), created.CategoryID, "the service assigns the default category")
	assert.Equal(t, "feed", created.ResolvedParseType)

	listed, err := app.ListSources(nil)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)

	update := sampleRequest()
	update.ID = created.ID
	update.Title = "renamed"
	update.MaxFetchNum = 7

	updated, err := app.UpdateSource(update)
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Title)
	assert.Equal(t, 7, updated.MaxFetchNum)

	require.NoError(t, app.SetSourceEnabled(created.ID, false))

	listed, err = app.ListSources(nil)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.False(t, listed[0].Enabled)

	require.NoError(t, app.DeleteSource(created.ID))

	empty, err = app.ListSources(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	assert.Error(t, app.DeleteSource(created.ID), "deleting twice reports not found")
}

func TestUpdateKeepsHiddenFields(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	created, err := app.CreateSource(sampleRequest())
	require.NoError(t, err)

	// the health columns are written by real fetches only; simulate one run.
	require.NoError(t, app.svc.UpdateSourceColumn(created.ID, "status", 2))
	require.NoError(t, app.svc.UpdateSourceColumn(created.ID, "error_info", "boom"))

	update := sampleRequest()
	update.ID = created.ID
	update.Title = "renamed again"

	updated, err := app.UpdateSource(update)
	require.NoError(t, err)
	assert.Equal(t, "renamed again", updated.Title)
	assert.Equal(t, 2, updated.Status, "an edit must not reset the fetch status")
	assert.Equal(t, "boom", updated.ErrorInfo, "an edit must not clear the error info")
}

func TestPreviewParsesWithoutWriting(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(sampleAtom))
	}))
	defer server.Close()

	app := newTestApp(t)

	req := sampleRequest()
	req.URL = server.URL

	created, err := app.CreateSource(req)
	require.NoError(t, err)

	articles, err := app.PreviewSource(created.ID, "")
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "Hello Preview", articles[0].Title)
	assert.Equal(t, "https://example.com/one", articles[0].URL)

	listed, err := app.ListSources(nil)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, 0, listed[0].Status, "a preview leaves the source health untouched")
	assert.Empty(t, listed[0].ErrorInfo)
}

func TestStartupFailureSurfaces(t *testing.T) {
	t.Parallel()

	app := newAppWithHome(filepath.Join(t.TempDir(), "missing", "nested"))

	assert.NotEmpty(t, app.StartupError())

	_, err := app.ListSources(nil)
	assert.ErrorIs(t, err, ErrNotReady)
}
