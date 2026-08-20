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

package compose

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/mcp"
)

// ServeCommand is the sub command that puts an informer executable into tool
// server mode for a composing conversation.
//
// It is a second sub command rather than a mode flag on the diagnosis one
// because the two carry different session documents and offer different tools. A
// single entry point would have to decide which it was on every tool call, and
// the failure mode of getting that wrong - offering get_source on a draft that
// has no id - is nonsense the agent has no way to recognize as nonsense.
const ServeCommand = "mcp-compose"

// serveDirFlag names the run directory on that command line.
const serveDirFlag = "--dir"

// ServeArgs reports whether a process argument list asks for tool server mode,
// and which run directory it names. args excludes the executable itself.
func ServeArgs(args []string) (string, bool) {
	if len(args) < 3 || args[0] != ServeCommand || args[1] != serveDirFlag {
		return "", false
	}

	return args[2], true
}

// ServeStdio runs the tool server over this process's own stdin and stdout.
//
// It first moves the global logger off stdout, because stdout is the protocol:
// one line of ordinary log output in the middle of a json-rpc stream is a parse
// error on the other side, and every fetch informer makes logs. Stderr is read
// by the agent command line, so nothing is lost by moving it there.
func ServeStdio(ctx context.Context, dir, version string) error {
	logger.SetOutput(os.Stderr)

	return Serve(ctx, dir, version, os.Stdin, os.Stdout)
}

// Serve runs the composing tool server of one session directory over the given
// stdio pair, and returns when the client closes it.
//
// One of these is launched per turn, not per conversation: the agent command
// line starts a fresh child every time it is invoked. That is why the tools hold
// no state worth keeping - the page cache inside one child is a within-turn
// optimization, and everything that has to survive the turn is either in the
// transcript or in the proposal document.
func Serve(ctx context.Context, dir, version string, in io.Reader, out io.Writer) error {
	session, err := ReadSession(dir)
	if err != nil {
		return err
	}

	server, err := mcp.NewServer(ServerName, version, Tools(session)...)
	if err != nil {
		return fmt.Errorf("build compose server: %w", err)
	}

	return server.Serve(ctx, in, out)
}
