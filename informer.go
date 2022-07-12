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

	"github.com/vogo/logger"
	"github.com/wongoo/informer/internal/feed"
	"github.com/wongoo/informer/internal/inform"
)

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	var op string

	if len(os.Args) > 1 {
		logger.SetLevel(logger.LevelDebug)

		op = os.Args[1]
		if op == "feed" {
			feed.InitFeedDB(exeDir)
			feed.Operate(os.Args[2:])

			return
		}
	}

	inform.Inform(exeDir, op)
}
