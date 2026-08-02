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
	"fmt"
	"os"
	"path/filepath"

	"github.com/vogo/informer/internal/home"
	"github.com/vogo/informer/internal/service"
)

// App is the desktop binding layer. It is the only bridge between the Vue
// frontend and the shared service layer: both entries resolve the same data
// directory through internal/home and reach the business through
// service.Service. The frontend only ever sees the flat DTOs defined next to
// the bound methods, never a persistence model.
type App struct {
	// ctx is the wails runtime context, stored per the wails binding pattern
	// so bound methods can reach runtime calls when they later need them.
	ctx context.Context //nolint:containedctx //wails stores its context this way.

	svc     *service.Service
	homeDir string

	// initErr carries a data directory or service startup failure into the
	// running window so the UI can show it instead of a blank page.
	initErr error
}

// newApp resolves the single active data directory and builds the service
// bound to it. A failure never aborts the process: the returned App carries
// the error, the window still opens, and StartupError reports the reason.
func newApp() *App {
	exePath, err := os.Executable()
	if err != nil {
		return &App{initErr: err}
	}

	homeDir, err := home.Init(filepath.Dir(exePath))
	if err != nil {
		return &App{initErr: err}
	}

	return newAppWithHome(homeDir)
}

// newAppWithHome builds the app on one explicit data directory, the seam the
// binding tests use to run against a throw away directory.
func newAppWithHome(homeDir string) *App {
	svc, err := service.New(homeDir)
	if err != nil {
		return &App{initErr: err}
	}

	return &App{svc: svc, homeDir: homeDir}
}

// Version returns the build version the UI shows and the smoke tests assert.
func (a *App) Version() string {
	return version
}

// HomeDir returns the active data directory so the UI states which data it
// manages; it is empty when startup failed before the directory resolved.
func (a *App) HomeDir() string {
	return a.homeDir
}

// StartupError returns the startup failure message, or an empty string when
// the data directory and the service came up cleanly.
func (a *App) StartupError() string {
	if a.initErr == nil {
		return ""
	}

	return a.initErr.Error()
}

// startup records the wails context once the window exists.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ready guards every data touching binding: a startup failure is reported on
// each call, wrapped in ErrNotReady so the frontend recognizes it, instead of
// dereferencing a service that was never built.
func (a *App) ready() error {
	if a.initErr == nil {
		return nil
	}

	return fmt.Errorf("%w: %w", ErrNotReady, a.initErr)
}
