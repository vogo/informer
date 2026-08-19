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

// Package runlog carries the log of one single run to whoever asked for it.
//
// The global logger writes to one process wide stream, so a line it prints
// cannot be attributed to the fetch that produced it: a scheduled inform run and
// a test fetch started from the window interleave freely. A Sink is the missing
// half - it is handed down the call chain of exactly one run, and every helper
// here writes to the global logger first and to that sink second. Nothing about
// the existing stdout behavior changes; a caller that passes no sink gets
// exactly what it got before.
package runlog

import (
	"fmt"
	"strings"
	"time"

	"github.com/vogo/logger"
)

// Levels of one recorded line. They are the strings the desktop page styles on,
// so they stay lowercase and stable.
const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Entry is one recorded line of a run.
type Entry struct {
	// Time is when the line was written, in unix milliseconds. Millisecond
	// resolution is what makes the gaps of an agent run readable.
	Time int64 `json:"time"`

	// Level is one of the Level constants.
	Level string `json:"level"`

	// Text is the rendered message, without timestamp or level prefix.
	Text string `json:"text"`
}

// Sink receives the entries of one run as they happen.
//
// An implementation may be called from more than one goroutine - an agent run
// reads its stdout and its stderr at the same time - and has to guard itself.
type Sink interface {
	Write(entry Entry)
}

// FuncSink adapts a plain function to a Sink.
type FuncSink func(entry Entry)

// Write passes the entry to the wrapped function, and does nothing when there
// is none, so a zero FuncSink is as harmless as a nil Sink.
func (f FuncSink) Write(entry Entry) {
	if f == nil {
		return
	}

	f(entry)
}

// Infof records an ordinary step of the run.
func Infof(sink Sink, format string, a ...any) {
	text := fmt.Sprintf(format, a...)

	logger.Info(text)
	write(sink, LevelInfo, text)
}

// Warnf records something the run survived but the user should see.
func Warnf(sink Sink, format string, a ...any) {
	text := fmt.Sprintf(format, a...)

	logger.Warn(text)
	write(sink, LevelWarn, text)
}

// Errorf records the reason a run produced nothing usable.
func Errorf(sink Sink, format string, a ...any) {
	text := fmt.Sprintf(format, a...)

	logger.Error(text)
	write(sink, LevelError, text)
}

// Log records an already rendered line at an explicit level. It is what an
// adapter over another package's callback uses, where the format string was
// applied on the other side.
func Log(sink Sink, level, text string) {
	switch level {
	case LevelError:
		logger.Error(text)
	case LevelWarn:
		logger.Warn(text)
	default:
		level = LevelInfo

		logger.Info(text)
	}

	write(sink, level, text)
}

// write hands one entry to a sink, skipping a caller that asked for no capture.
func write(sink Sink, level, text string) {
	if sink == nil {
		return
	}

	sink.Write(Entry{
		Time:  time.Now().UnixMilli(),
		Level: level,
		Text:  text,
	})
}

// Truncate shortens a quoted payload - a response body, an agent answer - so one
// oversized document cannot bury the rest of the log. It cuts on rune
// boundaries, so a chinese page is never sliced into invalid utf-8.
//
// A limit of zero or less leaves the text alone.
//
//nolint:gosmopolitan //informer is a chinese product; the notice speaks the user's language.
func Truncate(text string, limit int) string {
	trimmed := strings.TrimSpace(text)
	if limit <= 0 {
		return trimmed
	}

	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}

	return string(runes[:limit]) + fmt.Sprintf("...(共 %d 字符，已截断)", len(runes))
}
