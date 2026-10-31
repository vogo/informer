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
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// The GitHub provider permits releases without a checksum sidecar. Our
// release contract requires SHA256, so fail closed before downloading.
var errMissingUpdateChecksum = errors.New("missing valid SHA256 checksum")

type verifiedProvider struct{ updater.Provider }

func (p verifiedProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	rel, err := p.Provider.Check(ctx, req)
	if err != nil || rel == nil {
		return rel, err
	}
	v := rel.Verification
	if v == nil || v.DigestAlgo != "sha256" || len(v.Digest) != sha256.Size {
		return nil, fmt.Errorf("release %s: %w", rel.Version, errMissingUpdateChecksum)
	}

	return rel, nil
}

type backgroundUpdater interface {
	State() updater.State
	CheckAndInstall(ctx context.Context) error
}

func runUpdateChecks(ctx context.Context, interval time.Duration, u backgroundUpdater, report func(error)) {
	check := func() {
		if ctx.Err() != nil {
			return
		}
		switch u.State() {
		case updater.StateReady, updater.StateChecking, updater.StateDownloading, updater.StateVerifying, updater.StateInstalling:
			return
		case updater.StateUnconfigured, updater.StateIdle, updater.StateUpToDate, updater.StateAvailable, updater.StateError:
		}
		err := u.CheckAndInstall(ctx)
		if err != nil && ctx.Err() == nil {
			report(err)
		}
	}
	check()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

// Probe the parent, where the helper needs to create its backup and replace
// the application. Never quit into a known read-only installation.
func checkUpdateLocation(executable string) error {
	target := executable
	for dir := filepath.Dir(executable); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if strings.HasSuffix(dir, ".app") {
			target = dir

			break
		}
	}
	probe, err := os.CreateTemp(filepath.Dir(target), ".informer-update-probe-*")
	if err != nil {
		return fmt.Errorf("installation directory is not writable; move the app to a writable directory or install the update manually: %w", err)
	}
	name := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}

	return removeErr
}
