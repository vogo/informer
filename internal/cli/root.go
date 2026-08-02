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

package cli

import (
	"os"
	"path/filepath"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/home"
	"github.com/vogo/informer/internal/service"
)

// Run executes the informer command line with the full process arguments. It
// resolves the active data directory, builds the service bound to it and
// dispatches the requested command.
//
// Both executable entries - the root package kept for `go install
// github.com/vogo/informer@master` and cmd/informer - call this one function,
// so their behavior cannot fork. Errors are returned, never printed or
// turned into process exits here; each entry point decides how to report
// them, keeping this package free of presentation choices.
func Run(args []string) error {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	// the active data directory is resolved once and migrated from the legacy
	// executable directory, so a Finder launched app finds the same data as crontab.
	homeDir, err := home.Init(exeDir)
	if err != nil {
		return err
	}

	var action string

	if len(args) > 1 {
		logger.SetLevel(logger.LevelDebug)

		action = args[1]
	}

	svc, err := service.New(homeDir)
	if err != nil {
		return err
	}

	if action == "feed" {
		return Feed(svc, args[2:])
	}

	_, err = svc.TriggerInform(action)

	return err
}
