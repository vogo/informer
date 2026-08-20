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

package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/mcp"
)

// objectSchema is the argument schema of a tool that takes a plain object.
func objectSchema() map[string]any {
	return map[string]any{"type": "object"}
}

// echoTool answers with whatever it was handed, so a test can assert that the
// arguments survived the round trip byte for byte.
func echoTool() mcp.Tool {
	return mcp.Tool{
		Name:        "echo",
		Description: "returns its arguments",
		InputSchema: objectSchema(),
		Handler: func(_ context.Context, arguments json.RawMessage) (string, error) {
			return string(arguments), nil
		},
	}
}

// errBadRegex is what failingTool reports; a static error keeps the test
// honest about the shape production code is held to.
var errBadRegex = errors.New("regex did not compile")

// failingTool always fails, which is how a tool level failure is asserted to
// stay inside the result rather than becoming a protocol error.
func failingTool() mcp.Tool {
	return mcp.Tool{
		Name:        "boom",
		Description: "always fails",
		InputSchema: objectSchema(),
		Handler: func(context.Context, json.RawMessage) (string, error) {
			return "", errBadRegex
		},
	}
}

// exchange runs a session over the given request lines and returns the answers.
func exchange(t *testing.T, tools []mcp.Tool, requests ...string) []map[string]any {
	t.Helper()

	server, err := mcp.NewServer("informer-test", "v0", tools...)
	require.NoError(t, err)

	var out strings.Builder

	err = server.Serve(context.Background(), strings.NewReader(strings.Join(requests, "\n")+"\n"), &out)
	require.NoError(t, err)

	var answers []map[string]any

	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var frame map[string]any

		require.NoError(t, json.Unmarshal([]byte(line), &frame))

		answers = append(answers, frame)
	}

	return answers
}

func TestNewServerRefusesUnusableTools(t *testing.T) {
	t.Parallel()

	_, err := mcp.NewServer("x", "v0", mcp.Tool{Name: "", Handler: echoTool().Handler})
	require.Error(t, err)

	_, err = mcp.NewServer("x", "v0", mcp.Tool{Name: "no_handler"})
	require.Error(t, err)

	_, err = mcp.NewServer("x", "v0", echoTool(), echoTool())
	require.Error(t, err, "a duplicate name would make one of the two unreachable")
}

func TestInitializeAnnouncesToolsOnly(t *testing.T) {
	t.Parallel()

	answers := exchange(t, []mcp.Tool{echoTool()},
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)

	require.Len(t, answers, 1)

	result, ok := answers[0]["result"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, mcp.ProtocolVersion, result["protocolVersion"])

	capabilities, ok := result["capabilities"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, capabilities, "tools")

	info, ok := result["serverInfo"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "informer-test", info["name"])
}

func TestToolsListCarriesSchema(t *testing.T) {
	t.Parallel()

	answers := exchange(t, []mcp.Tool{echoTool()},
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)

	require.Len(t, answers, 1)

	result, ok := answers[0]["result"].(map[string]any)
	require.True(t, ok)

	listed, ok := result["tools"].([]any)
	require.True(t, ok)
	require.Len(t, listed, 1)

	tool, ok := listed[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "echo", tool["name"])
	require.Contains(t, tool, "inputSchema")
	require.NotContains(t, tool, "Handler", "a function is not part of the wire shape")
}

func TestToolsCallReturnsTextContent(t *testing.T) {
	t.Parallel()

	answers := exchange(t, []mcp.Tool{echoTool()},
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"a":1}}}`)

	require.Len(t, answers, 1)

	result, ok := answers[0]["result"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, result["isError"])

	content, ok := result["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)

	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", first["type"])
	require.JSONEq(t, `{"a":1}`, first["text"].(string))
}

// A failing tool has to stay a result: the model reads the reason and tries
// again, while a protocol error would end the session it is in the middle of.
func TestFailingToolIsReportedInsideTheResult(t *testing.T) {
	t.Parallel()

	answers := exchange(t, []mcp.Tool{failingTool()},
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"boom","arguments":{}}}`)

	require.Len(t, answers, 1)
	require.NotContains(t, answers[0], "error")

	result, ok := answers[0]["result"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, result["isError"])

	content, ok := result["content"].([]any)
	require.True(t, ok)

	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Contains(t, first["text"], "regex did not compile")
}

func TestUnknownToolAndUnknownMethodAreProtocolErrors(t *testing.T) {
	t.Parallel()

	answers := exchange(t, []mcp.Tool{echoTool()},
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"resources/list"}`)

	require.Len(t, answers, 2)

	for _, answer := range answers {
		failure, ok := answer["error"].(map[string]any)
		require.True(t, ok)
		require.InDelta(t, -32601, failure["code"], 0)
	}
}

// A notification carries no id and must be answered with nothing at all.
func TestNotificationIsNotAnswered(t *testing.T) {
	t.Parallel()

	answers := exchange(t, []mcp.Tool{echoTool()},
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":7,"method":"ping"}`)

	require.Len(t, answers, 1, "only the ping is answered")
	require.InDelta(t, 7, answers[0]["id"], 0)
}

// A garbled frame is answered and the session continues: one bad line from a
// chatty client must not cost the run.
func TestMalformedLineDoesNotEndTheSession(t *testing.T) {
	t.Parallel()

	answers := exchange(t, []mcp.Tool{echoTool()},
		`{not json`,
		`{"jsonrpc":"2.0","id":8,"method":"ping"}`)

	require.Len(t, answers, 2)

	failure, ok := answers[0]["error"].(map[string]any)
	require.True(t, ok)
	require.InDelta(t, -32700, failure["code"], 0)

	require.Contains(t, answers[1], "result")
}
