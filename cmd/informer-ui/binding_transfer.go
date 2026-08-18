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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/vogo/informer/internal/service"
)

// exportFileExt is the extension of an export file, appended when the platform
// file dialog returns a name without one.
const exportFileExt = ".json"

// exportFilterName and exportFilterPattern describe the export file type in the
// native dialogs of both directions. The labels are chinese like the rest of the
// interface: they are shown by the platform dialog, which the page cannot label
// itself.
const (
	exportFilterName    = "JSON 文件" //nolint:gosmopolitan //user facing label of a chinese ui.
	exportFilterPattern = "*.json"
)

// SourceExportResultDTO reports one export. A canceled dialog is a normal
// outcome rather than an error: the page says so and leaves everything as is.
type SourceExportResultDTO struct {
	Canceled bool   `json:"canceled"`
	Path     string `json:"path"`
	Total    int    `json:"total"`
}

// SourceImportResultDTO reports one import, entry by entry. Failed entries are
// listed in Errors instead of aborting the run, so a single unusable record in
// a long file never costs the user every other one.
type SourceImportResultDTO struct {
	Canceled bool     `json:"canceled"`
	Path     string   `json:"path"`
	Total    int      `json:"total"`
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors"`
}

// ExportSourcesToFile writes every stored subscription to a file the user picks.
// The document carries the whole configuration of each subscription and the
// category by name; the database ids and the fetch health stay behind, because
// neither means anything on another installation.
func (a *App) ExportSourcesToFile() (*SourceExportResultDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	path, err := promptExportPath()
	if err != nil {
		return nil, err
	}

	if path == "" {
		return &SourceExportResultDTO{Canceled: true}, nil
	}

	total, err := a.writeSourceExport(path)
	if err != nil {
		return nil, err
	}

	return &SourceExportResultDTO{Path: path, Total: total}, nil
}

// ImportSourcesFromFile merges an export file the user picks into the stored
// subscriptions: a subscription already known by url - or by title when it has
// no url - is overwritten, an unknown one is appended, and nothing is deleted.
func (a *App) ImportSourcesFromFile() (*SourceImportResultDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	path, err := promptImportPath()
	if err != nil {
		return nil, err
	}

	if path == "" {
		return &SourceImportResultDTO{Canceled: true}, nil
	}

	result, err := a.readSourceImport(path)
	if err != nil {
		return nil, err
	}

	return &SourceImportResultDTO{
		Path:    path,
		Total:   result.Total,
		Created: result.Created,
		Updated: result.Updated,
		Failed:  result.Failed,
		Errors:  result.Errors,
	}, nil
}

// writeSourceExport renders the export document and stores it at path, reporting
// how many subscriptions it holds. It is the whole export minus the dialog, so
// the file format is exercised without a running window.
func (a *App) writeSourceExport(path string) (int, error) {
	doc, err := a.svc.ExportSources()
	if err != nil {
		return 0, err
	}

	data, err := service.MarshalSourceExport(doc)
	if err != nil {
		return 0, err
	}

	err = os.WriteFile(path, data, 0o600)
	if err != nil {
		return 0, fmt.Errorf("write export file %q: %w", path, err)
	}

	return len(doc.Sources), nil
}

// readSourceImport reads one export file and applies it. Like writeSourceExport
// it is the operation minus the dialog.
func (a *App) readSourceImport(path string) (*service.ImportSourcesResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read import file %q: %w", path, err)
	}

	return a.svc.ImportSourcesJSON(data)
}

// promptExportPath asks for the destination file. An empty path means the user
// canceled, which every caller reports as such instead of as a failure.
func promptExportPath() (string, error) {
	app := application.Get()
	if app == nil {
		return "", ErrApplicationNotRunning
	}

	// the save dialog carries its title only through the options struct; the
	// fluent setters of this wails version cannot set it.
	dialog := app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:                "导出订阅配置", //nolint:gosmopolitan //user facing label of a chinese ui.
		Filename:             defaultExportFilename(),
		CanCreateDirectories: true,
		Filters:              []application.FileFilter{{DisplayName: exportFilterName, Pattern: exportFilterPattern}},
	})

	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}

	return withExportExt(path), nil
}

// promptImportPath asks for the file to read. An empty path means the user canceled.
func promptImportPath() (string, error) {
	app := application.Get()
	if app == nil {
		return "", ErrApplicationNotRunning
	}

	path, err := app.Dialog.OpenFile().
		SetTitle("导入订阅配置"). //nolint:gosmopolitan //user facing label of a chinese ui.
		CanChooseFiles(true).
		AddFilter(exportFilterName, exportFilterPattern).
		PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("open file dialog: %w", err)
	}

	return path, nil
}

// defaultExportFilename is the name the save dialog starts with, dated so a
// second export never silently overwrites the first.
func defaultExportFilename() string {
	return "informer-sources-" + time.Now().Format("20060102") + exportFileExt
}

// withExportExt appends the json extension to a name typed without one, so a
// dialog that does not add the filter extension itself still writes a .json file.
func withExportExt(path string) string {
	if path == "" || strings.EqualFold(filepath.Ext(path), exportFileExt) {
		return path
	}

	return path + exportFileExt
}
