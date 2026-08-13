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
// service layer in a Wails v3 window: the frontend talks only to the flat DTO
// bindings of App, never to the database. Unlike the CGO free CLI, this
// entry requires CGO because it links the native WebView and mattn/go-sqlite3.
package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"

	"github.com/vogo/informer/internal/httpx"
)

// appTitle is the product name shown in the window chrome, kept in one place
// so the surfaces never diverge.
const appTitle = "informer"

// githubRepo is the Releases source the in-app updater polls.
const githubRepo = "vogo/informer"

// checksumAsset is the fixed checksums filename published with each release.
const checksumAsset = "SHA256SUMS"

// updaterHTTPTimeout covers GitHub API checks and full release archive downloads.
const updaterHTTPTimeout = 10 * time.Minute

const (
	platformDarwin  = "darwin"
	platformWindows = "windows"
	platformLinux   = "linux"
	archAMD64       = "amd64"
	archARM64       = "arm64"
)

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
	// It also runs before application.New, which may divert into updater
	// helper mode when restarting after a download.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version) //nolint:forbidigo //version is a deliberate stdout contract.

		return
	}

	desktop := newApp()

	app := application.New(application.Options{
		Name:        appTitle,
		Description: appTitle + " desktop " + version,
		Services: []application.Service{
			application.NewService(desktop),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	err := initUpdater(app)
	if err != nil {
		fmt.Fprintln(os.Stderr, "informer-ui: updater:", err)
	}

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     appTitle,
		Width:     1024,
		Height:    768,
		MinWidth:  760,
		MinHeight: 520,
		URL:       "/",
	})

	err = app.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "informer-ui:", err)
		os.Exit(1)
	}
}

// initUpdater wires the official Wails updater against GitHub Releases.
// Development builds (version "dev") skip network checks entirely.
func initUpdater(app *application.App) error {
	if version == "dev" || strings.TrimSpace(version) == "" {
		return nil
	}

	gh, err := github.New(github.Config{
		Repository:    githubRepo,
		ChecksumAsset: checksumAsset,
		AssetMatcher:  informerAssetMatcher,
		HTTPClient:    httpx.NewClient(updaterHTTPTimeout),
	})
	if err != nil {
		return err
	}

	current := strings.TrimPrefix(version, "v")

	err = app.Updater.Init(updater.Config{
		CurrentVersion: current,
		Providers:      []updater.Provider{gh},
		Window:         updater.WindowNone,
		CheckInterval:  24 * time.Hour,
	})
	if err != nil {
		return err
	}

	// CheckInterval waits for the first tick; kick one check on startup so a
	// freshly opened window still learns about a release published today.
	go func() {
		checkErr := app.Updater.CheckAndInstall(context.Background())
		if checkErr != nil {
			app.Logger.Error("update check", "error", checkErr)
		}
	}()

	return nil
}

// informerAssetMatcher picks the updater-consumable archive for each OS:
// macOS universal .app.zip, Windows plain .zip (not the NSIS setup), Linux
// .tar.gz. Installer / dmg / deb assets stay available for manual download.
func informerAssetMatcher(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	plat := strings.ToLower(req.Platform)
	arch := strings.ToLower(req.Arch)

	for i, a := range assets {
		name := strings.ToLower(a.Name)
		if matchUpdateAsset(plat, arch, name) {
			return i
		}
	}

	return -1
}

func matchUpdateAsset(plat, arch, name string) bool {
	switch plat {
	case platformDarwin:
		return matchDarwinAsset(name)
	case platformWindows:
		return matchWindowsAsset(name, arch)
	case platformLinux:
		return matchLinuxAsset(name, arch)
	default:
		return false
	}
}

func matchDarwinAsset(name string) bool {
	if !strings.Contains(name, "darwin-universal") {
		return false
	}

	return strings.HasSuffix(name, ".app.zip") ||
		(strings.HasSuffix(name, ".zip") && !strings.Contains(name, "dmg"))
}

func matchWindowsAsset(name, arch string) bool {
	return strings.Contains(name, platformWindows) &&
		assetHasArch(name, arch) &&
		strings.HasSuffix(name, ".zip") &&
		!strings.Contains(name, "setup") &&
		!strings.Contains(name, "installer")
}

func matchLinuxAsset(name, arch string) bool {
	return strings.Contains(name, platformLinux) &&
		assetHasArch(name, arch) &&
		(strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz"))
}

func assetHasArch(name, arch string) bool {
	if arch == "" || strings.Contains(name, arch) {
		return true
	}

	if arch == archAMD64 {
		return strings.Contains(name, "x86_64") || strings.Contains(name, "x64")
	}

	if arch == archARM64 {
		return strings.Contains(name, "aarch64")
	}

	return false
}
