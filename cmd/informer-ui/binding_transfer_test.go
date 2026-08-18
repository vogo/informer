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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/service"
)

func TestExportFileRestoresEverySubscriptionElsewhere(t *testing.T) {
	t.Parallel()

	source := newTestApp(t)

	tech, err := source.CreateCategory(&SaveCategoryRequest{Name: techName})
	require.NoError(t, err)

	req := sampleRequest()
	req.CategoryID = tech.ID
	req.Weight = 12

	created, err := source.CreateSource(req)
	require.NoError(t, err)

	// a disabled subscription has to come back disabled on the other side.
	require.NoError(t, source.SetSourceEnabled(created.ID, false))

	path := filepath.Join(t.TempDir(), "sources.json")

	total, err := source.writeSourceExport(path)
	require.NoError(t, err)
	assert.Equal(t, 1, total)

	target := newTestApp(t)

	result, err := target.readSourceImport(path)
	require.NoError(t, err)
	assert.Equal(t, &service.ImportSourcesResult{Total: 1, Created: 1}, result)

	restored, err := target.ListSources(nil)
	require.NoError(t, err)
	require.Len(t, restored, 1)
	assert.Equal(t, req.URL, restored[0].URL)
	assert.Equal(t, int64(12), restored[0].Weight)
	assert.False(t, restored[0].Enabled)

	// the category traveled by name and was created on the way in.
	categories, err := target.ListCategories()
	require.NoError(t, err)
	assert.Equal(t, []string{defaultName, techName}, categoryNames(categories))
	assert.Equal(t, restored[0].CategoryID, categoryID(categories, techName))
}

func TestImportOverwritesTheMatchedSubscriptionAndAppendsTheRest(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	req := sampleRequest()
	req.Title = "old title"

	stored, err := app.CreateSource(req)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "sources.json")
	writeExportFile(t, path, `{
	  "version": 1,
	  "sources": [
	    {"title": "new title", "url": "`+req.URL+`", "parse_type": "feed", "weight": 5, "enabled": true},
	    {"title": "another", "url": "https://example.com/other.xml", "parse_type": "feed", "enabled": true}
	  ]
	}`)

	result, err := app.readSourceImport(path)
	require.NoError(t, err)
	assert.Equal(t, &service.ImportSourcesResult{Total: 2, Created: 1, Updated: 1}, result)

	listed, err := app.ListSources(nil)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, stored.ID, listed[0].ID)
	assert.Equal(t, "new title", listed[0].Title)
	assert.Equal(t, int64(5), listed[0].Weight)
	assert.Equal(t, "another", listed[1].Title)
}

func TestImportReportsAnUnusableFileAndAnUnusableEntry(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	dir := t.TempDir()

	broken := filepath.Join(dir, "broken.json")
	writeExportFile(t, broken, "not json at all")

	_, err := app.readSourceImport(broken)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = app.readSourceImport(filepath.Join(dir, "missing.json"))
	require.ErrorIs(t, err, os.ErrNotExist)

	partial := filepath.Join(dir, "partial.json")
	writeExportFile(t, partial, `{"sources": [
	  {"title": "", "url": "", "enabled": true},
	  {"title": "good", "url": "https://example.com/good.xml", "parse_type": "feed", "enabled": true}
	]}`)

	result, err := app.readSourceImport(partial)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Failed)
	require.Len(t, result.Errors, 1)

	listed, err := app.ListSources(nil)
	require.NoError(t, err)
	assert.Len(t, listed, 1)
}

func TestTransferBindingsReportAStartupFailure(t *testing.T) {
	t.Parallel()

	app := newAppWithHome(filepath.Join(t.TempDir(), "missing", "nested"))
	require.NotEmpty(t, app.StartupError())

	_, err := app.ExportSourcesToFile()
	require.ErrorIs(t, err, ErrNotReady)

	_, err = app.ImportSourcesFromFile()
	require.ErrorIs(t, err, ErrNotReady)
}

func TestExportFileNameAlwaysCarriesTheJSONExtension(t *testing.T) {
	t.Parallel()

	assert.True(t, strings.HasSuffix(defaultExportFilename(), ".json"))
	assert.Equal(t, "/tmp/a.json", withExportExt("/tmp/a"))
	assert.Equal(t, "/tmp/a.json", withExportExt("/tmp/a.json"))
	assert.Equal(t, "/tmp/a.JSON", withExportExt("/tmp/a.JSON"))
	// an empty path is the canceled dialog and stays empty.
	assert.Empty(t, withExportExt(""))
}

func writeExportFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func categoryID(categories []*CategoryDTO, name string) int64 {
	for _, category := range categories {
		if category.Name == name {
			return category.ID
		}
	}

	return 0
}
