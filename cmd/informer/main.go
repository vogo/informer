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

// Command informer is the formal command line entry of informer. It shares the
// exact startup function with the root package compat entry, so `go build
// ./cmd/informer` and `go install github.com/vogo/informer@master` behave
// identically. The CLI stays CGO free at build time.
package main

import (
	"os"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/cli"
)

func main() {
	err := cli.Run(os.Args)
	if err != nil {
		logger.Fatal(err)
	}
}
