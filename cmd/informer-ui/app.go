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
	"sync"

	"github.com/vogo/informer/internal/home"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/scheduler"
	"github.com/vogo/informer/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the desktop binding layer. It is the only bridge between the Vue
// frontend and the shared service layer: both entries resolve the same data
// directory through internal/home and reach the business through
// service.Service. The frontend only ever sees the flat DTOs defined next to
// the bound methods, never a persistence model.
type App struct {
	svc     *service.Service
	homeDir string

	// informMu serializes the inform runs of this process: the scheduled fire
	// and a manual push share one pipeline, and two runs at once would fetch,
	// score and deliver the very same articles twice.
	informMu sync.Mutex

	// sched runs the configured daily push while the window is open; nil when
	// startup failed or before the window exists.
	sched *scheduler.Scheduler

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

// ServiceStartup starts the desktop scheduler once the Wails runtime is up.
// A broken data directory still shows in the window: the scheduler never runs
// on a service that was not built, and this method never aborts startup.
func (a *App) ServiceStartup(context.Context, application.ServiceOptions) error {
	if a.initErr != nil {
		return nil
	}

	a.sched = scheduler.New(
		func() (bool, string, error) {
			schedule, err := a.svc.ReadScheduleConfig()
			if err != nil {
				return false, "", err
			}

			return schedule.Enabled, schedule.Time, nil
		},
		a.svc.ReadScheduleLastRunDate,
		a.svc.MarkScheduleLastRunDate,
		func() error {
			_, err := a.runInform()

			return err
		},
	)
	a.sched.Start()

	return nil
}

// ServiceShutdown stops the desktop scheduler when the application quits.
// An in-flight run is left to finish or die with the process; Stop never waits.
func (a *App) ServiceShutdown() error {
	if a.sched == nil {
		return nil
	}

	a.sched.Stop()

	return nil
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

// RestartToUpdate applies a staged update: spawn the helper, quit this
// process, swap the binary, and relaunch. It is a no-op error when no update
// has been downloaded yet.
func (a *App) RestartToUpdate() error {
	app := application.Get()
	if app == nil {
		return fmt.Errorf("application is not running")
	}

	return app.Updater.Restart(context.Background())
}

// UpdateState returns the updater lifecycle phase for the header badge
// ("idle", "downloading", "ready", …). Empty when the updater is disabled
// (dev builds).
func (a *App) UpdateState() string {
	app := application.Get()
	if app == nil {
		return ""
	}

	return string(app.Updater.State())
}

// runInform runs one full inform cycle under the process wide guard, so the
// scheduled fire and a manual push can never overlap. A caller that arrives
// while a run is in flight gets ErrInformRunning at once instead of queueing
// up: the daily content is idempotent but the bot message is not.
func (a *App) runInform() (*inform.Result, error) {
	if !a.informMu.TryLock() {
		return nil, ErrInformRunning
	}

	defer a.informMu.Unlock()

	return a.svc.TriggerInform("")
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
