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

// Package mcp is the smallest Model Context Protocol server informer needs.
//
// It speaks JSON-RPC 2.0 over stdio - one json document per line - and answers
// exactly the four methods a tool-only server has to answer: initialize, ping,
// tools/list and tools/call. Nothing here knows about subscriptions; a caller
// hands it a set of Tools and this package moves the bytes.
//
// Writing it by hand rather than pulling in an sdk is deliberate: the surface a
// tool-only server needs is this file, and informer's dependency list is the
// thing a desktop app ships to every user.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// ProtocolVersion is the revision this server implements. It is the widely
// deployed one, which is what a command line launched by informer speaks.
const ProtocolVersion = "2024-11-05"

// jsonRPCVersion is the version field every frame carries.
const jsonRPCVersion = "2.0"

// Fixed strings of the wire shape, named so the encoder and the tests cannot
// disagree about them.
const (
	capabilityTools = "tools"
	contentTypeText = "text"
)

// JSON-RPC methods this server answers. Anything else is refused with
// codeMethodNotFound rather than silently ignored.
const (
	methodInitialize  = "initialize"
	methodPing        = "ping"
	methodToolsList   = "tools/list"
	methodToolsCall   = "tools/call"
	methodInitialized = "notifications/initialized"
)

// JSON-RPC error codes used here, from the specification's reserved range.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// maxLineBytes bounds one incoming request. A tool call carries arguments, not
// documents, so a line past this is a broken peer rather than a large request.
const maxLineBytes = 4 << 20

// ErrToolNotFound marks a tools/call naming a tool this server does not have.
var ErrToolNotFound = errors.New("tool not found")

// Handler runs one tool call. The arguments arrive exactly as the caller sent
// them, so a tool owns its own decoding and its own validation.
//
// The returned text is what the model reads. An error is reported back as a
// failed tool result rather than as a protocol error: a model that is told
// "your regex did not compile" can fix it, while a transport level failure only
// ends the conversation.
type Handler func(ctx context.Context, arguments json.RawMessage) (string, error)

// Tool is one callable this server offers.
type Tool struct {
	// Name is how a model addresses the tool. It is namespaced by the client,
	// so it stays a plain lower_snake_case verb here.
	Name string `json:"name"`

	// Description tells the model when to reach for it. It is the only
	// documentation the model ever sees, so it is written for a reader who
	// knows nothing about informer.
	Description string `json:"description"`

	// InputSchema is the JSON Schema of the arguments object.
	InputSchema map[string]any `json:"inputSchema"`

	// Handler runs the call. A tool without one is refused at registration.
	Handler Handler `json:"-"`
}

// Server answers requests over one stdio pair.
type Server struct {
	name    string
	version string

	tools  []Tool
	byName map[string]Handler

	// writeMu serializes responses. Requests are answered in order today, but a
	// tool that ever answers asynchronously must not interleave half a line
	// into another response.
	writeMu sync.Mutex
	out     io.Writer
}

// NewServer builds a server offering the given tools.
// A tool without a name or without a handler is refused: an advertised tool
// that cannot run is worse than one that was never offered.
func NewServer(name, version string, tools ...Tool) (*Server, error) {
	server := &Server{
		name:    name,
		version: version,
		tools:   make([]Tool, 0, len(tools)),
		byName:  make(map[string]Handler, len(tools)),
	}

	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("%w: a tool has no name", errBadTool)
		}

		if tool.Handler == nil {
			return nil, fmt.Errorf("%w: tool %q has no handler", errBadTool, tool.Name)
		}

		if _, duplicate := server.byName[tool.Name]; duplicate {
			return nil, fmt.Errorf("%w: tool %q is registered twice", errBadTool, tool.Name)
		}

		server.byName[tool.Name] = tool.Handler
		server.tools = append(server.tools, tool)
	}

	return server, nil
}

// errBadTool marks a tool set a server cannot be built from.
var errBadTool = errors.New("invalid tool")

// Serve reads requests from in and writes answers to out until the stream ends
// or ctx is done.
//
// A malformed line is answered with a parse error and the loop continues: one
// bad frame from a chatty client is not a reason to end a session the model is
// in the middle of.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = out

	reader := bufio.NewReaderSize(in, 64<<10)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line, err := readLine(reader)
		if len(line) > 0 {
			s.handleLine(ctx, line)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("mcp read: %w", err)
		}
	}
}

// request is one incoming JSON-RPC frame. A notification carries no id and is
// answered with nothing at all, which is what the id being null encodes.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// response is one outgoing JSON-RPC frame; exactly one of Result and Error is set.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC failure of the transport itself, never of a tool.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handleLine answers one frame.
func (s *Server) handleLine(ctx context.Context, line []byte) {
	var req request

	err := json.Unmarshal(line, &req)
	if err != nil {
		s.writeError(nil, codeParseError, fmt.Sprintf("parse request: %v", err))

		return
	}

	// a notification is fire and forget; answering one is a protocol violation.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return
	}

	if req.Method == "" {
		s.writeError(req.ID, codeInvalidRequest, "request has no method")

		return
	}

	result, rpcErr := s.dispatch(ctx, &req)
	if rpcErr != nil {
		s.writeError(req.ID, rpcErr.Code, rpcErr.Message)

		return
	}

	s.write(response{JSONRPC: jsonRPCVersion, ID: req.ID, Result: result})
}

// dispatch runs one method and returns what its result field should carry.
func (s *Server) dispatch(ctx context.Context, req *request) (any, *rpcError) {
	switch req.Method {
	case methodInitialize:
		return s.initializeResult(), nil
	case methodPing:
		// the specification's ping answers with an empty object.
		return map[string]any{}, nil
	case methodToolsList:
		return map[string]any{capabilityTools: s.tools}, nil
	case methodToolsCall:
		return s.callTool(ctx, req.Params)
	case methodInitialized:
		// only reachable when a client sends it with an id, which some do.
		return map[string]any{}, nil
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "unknown method " + req.Method}
	}
}

// initializeResult states what this server is and what it can do. Only tools
// are advertised, because only tools are implemented.
func (s *Server) initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{capabilityTools: map[string]any{}},
		"serverInfo":      map[string]any{"name": s.name, "version": s.version},
	}
}

// callParams is the argument object of tools/call.
type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// callTool runs one tool.
//
// A tool that fails answers with isError rather than a protocol error, so the
// model reads the reason and gets another try; only a malformed call itself is
// a protocol error.
func (s *Server) callTool(ctx context.Context, params json.RawMessage) (any, *rpcError) {
	var call callParams

	if len(params) > 0 {
		err := json.Unmarshal(params, &call)
		if err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("parse params: %v", err)}
		}
	}

	if call.Name == "" {
		return nil, &rpcError{Code: codeInvalidParams, Message: "tools/call has no tool name"}
	}

	handler, found := s.byName[call.Name]
	if !found {
		return nil, &rpcError{Code: codeMethodNotFound, Message: fmt.Sprintf("%v: %s", ErrToolNotFound, call.Name)}
	}

	text, err := handler(ctx, call.Arguments)
	if err != nil {
		return toolResult(err.Error(), true), nil
	}

	return toolResult(text, false), nil
}

// toolResult renders one tool answer in the shape the protocol asks for.
func toolResult(text string, failed bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": contentTypeText, contentTypeText: text}},
		"isError": failed,
	}
}

// write emits one frame, ignoring a closed pipe: the client going away is the
// normal end of a session, not something to report into a stream nobody reads.
func (s *Server) write(frame response) {
	encoded, err := json.Marshal(frame)
	if err != nil {
		// a result that cannot be encoded is the server's own bug; say so on
		// the one channel that is still known to work.
		s.writeError(frame.ID, codeInternalError, fmt.Sprintf("encode result: %v", err))

		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, _ = s.out.Write(append(encoded, '\n'))
}

// writeError emits one failure frame.
func (s *Server) writeError(id json.RawMessage, code int, message string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}

	encoded, err := json.Marshal(response{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
	if err != nil {
		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, _ = s.out.Write(append(encoded, '\n'))
}

// readLine reads one frame, dropping the trailing newline and refusing a line
// past maxLineBytes so a broken peer cannot exhaust memory.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var collected []byte

	for {
		chunk, more, err := reader.ReadLine()
		collected = append(collected, chunk...)

		if len(collected) > maxLineBytes {
			return nil, fmt.Errorf("mcp: %w", errLineTooLong)
		}

		if err != nil {
			return collected, err
		}

		if !more {
			return collected, nil
		}
	}
}

// errLineTooLong marks a frame past the accepted size.
var errLineTooLong = errors.New("request line is too long")
