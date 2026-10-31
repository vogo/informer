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
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const (
	validChecksumMode   = "valid"
	olderVersionMode    = "older-version"
	sameVersionMode     = "same-version"
	missingChecksumMode = "missing"
	assetNameKey        = "name"
	corruptChecksumMode = "corrupt"
	downloadURLKey      = "browser_download_url"
)

type updateTestHost struct{}

func (updateTestHost) Emit(string, ...any) bool         { return true }
func (updateTestHost) OnEvent(string, func(any)) func() { return func() {} }
func (updateTestHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	panic("silent updater opened a window")
}
func (updateTestHost) Quit() { panic("background update must not quit") }

//nolint:funlen // Exercise the real provider, verification and archive staging together.
func TestUpdateDownloadVerification(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{validChecksumMode, missingChecksumMode, corruptChecksumMode, sameVersionMode, olderVersionMode} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			var archive bytes.Buffer
			zw := zip.NewWriter(&archive)
			f, err := zw.Create("informer-ui.exe")
			require.NoError(t, err)
			_, err = f.Write([]byte("new desktop binary"))
			require.NoError(t, err)
			require.NoError(t, zw.Close())
			digest := sha256.Sum256(archive.Bytes())
			if mode == corruptChecksumMode {
				digest = sha256.Sum256([]byte("other payload"))
			}
			var downloads atomic.Int32
			var base string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/vogo/informer/releases/latest":
					tag := "v9.9.9"
					if mode == sameVersionMode {
						tag = "v1.0.0"
					}
					if mode == olderVersionMode {
						tag = "v0.9.0"
					}
					assets := []map[string]any{{assetNameKey: testAssetWindowsZip, downloadURLKey: base + "/payload"}}
					if mode != missingChecksumMode {
						assets = append(assets, map[string]any{assetNameKey: checksumAsset, downloadURLKey: base + "/checksums"})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "assets": assets})
				case "/checksums":
					_, _ = fmt.Fprintf(w, "%x  %s\n", digest, testAssetWindowsZip)
				case "/payload":
					downloads.Add(1)
					_, _ = w.Write(archive.Bytes())
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			base = server.URL
			p, err := github.New(github.Config{Repository: githubRepo, BaseURL: base, ChecksumAsset: checksumAsset, AssetMatcher: informerAssetMatcher, HTTPClient: server.Client()})
			require.NoError(t, err)
			u := updater.New(updateTestHost{})
			require.NoError(t, u.Init(updater.Config{CurrentVersion: "1.0.0", Providers: []updater.Provider{verifiedProvider{p}}, Platform: platformWindows, Arch: archAMD64, Window: updater.WindowNone}))
			err = u.CheckAndInstall(context.Background())
			switch mode {
			case validChecksumMode:
				require.NoError(t, err)
				require.Equal(t, updater.StateReady, u.State())
				staged := u.DownloadedPath()
				defer os.RemoveAll(filepath.Dir(staged))
				contents, readErr := os.ReadFile(staged)
				require.NoError(t, readErr)
				require.Equal(t, "new desktop binary", string(contents))
			case sameVersionMode, olderVersionMode:
				require.NoError(t, err)
				require.Equal(t, updater.StateUpToDate, u.State())
				require.Zero(t, downloads.Load())
			default:
				require.Error(t, err)
				require.Equal(t, updater.StateError, u.State())
				require.Empty(t, u.DownloadedPath())
				if mode == missingChecksumMode {
					require.Zero(t, downloads.Load())
				}
			}
		})
	}
}

type pollingTestUpdater struct {
	calls atomic.Int32
	ready atomic.Bool
}

func (u *pollingTestUpdater) State() updater.State {
	if u.ready.Load() {
		return updater.StateReady
	}

	return updater.StateIdle
}

func (u *pollingTestUpdater) CheckAndInstall(context.Context) error {
	u.calls.Add(1)
	u.ready.Store(true)

	return nil
}

func TestUpdatePollingKeepsReadyAndStops(t *testing.T) {
	t.Parallel()
	u := &pollingTestUpdater{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); runUpdateChecks(ctx, time.Millisecond, u, func(error) {}) }()
	require.Eventually(t, u.ready.Load, time.Second, time.Millisecond)
	// Several ticks must leave the already downloaded payload alone.
	time.Sleep(15 * time.Millisecond)
	require.EqualValues(t, 1, u.calls.Load())
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poll loop did not stop")
	}
}

func TestUpdateLocationProbe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	app := filepath.Join(dir, "informer.app", "Contents", "MacOS")
	require.NoError(t, os.MkdirAll(app, 0o700))
	require.NoError(t, checkUpdateLocation(filepath.Join(app, "informer-ui")))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1) // Probe must be cleaned up beside the whole bundle.
	require.Error(t, checkUpdateLocation(filepath.Join(dir, missingChecksumMode, "informer-ui")))
}
