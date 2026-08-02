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

// Command informer-ui is the desktop entry of informer. It wraps the shared
// service layer in a Wails window: the frontend talks only to the flat DTO
// bindings of App, never to the database. Unlike the CGO free CLI, this
// entry requires CGO because it links the native WebView and mattn/go-sqlite3.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// appTitle is the product name shown in the window chrome and the macOS
// about panel, kept in one place so the surfaces never diverge.
const appTitle = "informer"

// version is injected at release time with
// -ldflags "-X main.version=${{ github.ref_name }}" so the UI and every
// packaged artifact carry the exact tag text. Development builds keep this
// explicit default so a shown version is never ambiguous.
var version = "dev" //nolint:gochecknoglobals //build time injection point.

//go:embed all:frontend/dist
var assets embed.FS //nolint:gochecknoglobals //wails asset server entry.

func main() {
	// the version sub command answers before any window or database exists,
	// so packaging smoke tests can verify the injected version headless.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version) //nolint:forbidigo //version is a deliberate stdout contract.

		return
	}

	app := newApp()

	err := wails.Run(&options.App{
		Title:     appTitle,
		Width:     1024,
		Height:    768,
		MinWidth:  760,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []any{
			app,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title:   appTitle,
				Message: appTitle + " desktop " + version,
			},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "informer-ui:", err)
		os.Exit(1)
	}
}
