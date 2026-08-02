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

	"github.com/vogo/informer/internal/cli"
	"github.com/vogo/informer/internal/home"
	"github.com/vogo/informer/internal/service"
	"github.com/vogo/logger"
)

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	// the active data directory is resolved once and migrated from the legacy
	// executable directory, so a Finder launched app finds the same data as crontab.
	homeDir, err := home.Init(exeDir)
	if err != nil {
		logger.Fatal(err)
	}

	var action string

	if len(os.Args) > 1 {
		logger.SetLevel(logger.LevelDebug)

		action = os.Args[1]
	}

	svc, err := service.New(homeDir)
	if err != nil {
		logger.Fatal(err)
	}

	if action == "feed" {
		if err = cli.Feed(svc, os.Args[2:]); err != nil {
			logger.Fatal(err)
		}

		return
	}

	if _, err = svc.TriggerInform(action); err != nil {
		logger.Fatal(err)
	}
}
