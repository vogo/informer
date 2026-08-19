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

// Package logbuf keeps the recent output of the global logger where a window
// can read it.
//
// The desktop app is launched from Finder or the start menu, so its stdout goes
// nowhere a user can look: a scheduled push that failed at three in the morning
// left no trace anybody could produce. This package taps the one stream every
// part of informer already writes to - Install replaces the logger's output with
// a tee - and keeps the last few thousand lines in memory, addressable by a
// cursor so a page can poll for what it has not seen yet.
//
// It is a tap, not a replacement: stdout keeps receiving exactly what it always
// did, and a process that never calls Install behaves as before. Nothing is
// written to disk; the buffer is the log of this run, and it dies with it.
//
// See internal/runlog for the other half - the log of one single fetch, handed
// to the caller that asked for that fetch. Both end up here, because runlog
// writes every line to the global logger too.
package logbuf

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/runlog"
)

// Levels one captured line can carry. Three of them are the runlog vocabulary,
// so a line recorded by a fetch keeps the same level name here that the test
// fetch panel styled on; debug is the level the global logger adds, reachable
// with the CLI's -d flag.
const (
	LevelDebug = "debug"
	LevelInfo  = runlog.LevelInfo
	LevelWarn  = runlog.LevelWarn
	LevelError = runlog.LevelError
)

// DefaultCapacity is how many lines the process wide buffer keeps. An inform run
// over a few dozen sources narrates a few hundred lines, so this holds several
// runs - enough to explain what a scheduled push did last night - while costing
// well under a megabyte.
const DefaultCapacity = 2000

// MaxLineRunes bounds one captured line. An agent source logs whole prompts and
// whole answers; without a cut, one of them would push every other line out of
// the buffer and freeze the page rendering it.
const MaxLineRunes = 2000

// stampLen is the width of the timestamp the global logger prefixes every line
// with, "2006/01/02 15:04:05.000". The tag follows after one space and is always
// four characters wide, which is what makes the prefix parseable by offset.
const stampLen = len("2006/01/02 15:04:05.000")

// tagLen is the fixed width of the level tag that follows the timestamp.
const tagLen = 4

// Entry is one captured line.
type Entry struct {
	// Seq numbers the lines of this process from 1 and never repeats, not even
	// after Clear. It is the cursor a poller passes back to Since, so a page can
	// ask for what it has not seen instead of re-reading the whole buffer.
	Seq int64 `json:"seq"`

	// Time is when the line was captured, in unix milliseconds.
	Time int64 `json:"time"`

	// Level is one of the Level constants.
	Level string `json:"level"`

	// Text is the message without its timestamp and level prefix.
	Text string `json:"text"`
}

// Snapshot is the answer to one Since call.
type Snapshot struct {
	// Entries are the matching lines, oldest first.
	Entries []Entry

	// Dropped counts the lines after the cursor the caller will never see,
	// because the buffer overwrote them or the limit cut them off. A page shows
	// it rather than pretending the log is continuous.
	Dropped int64

	// LatestSeq is the newest sequence the buffer has issued, and the cursor for
	// the next call. It moves even when the limit held entries back.
	LatestSeq int64

	// Capacity is how many lines the buffer keeps at most.
	Capacity int
}

// Buffer is a bounded ring of captured lines, safe for concurrent use.
//
// It implements io.Writer so the global logger can write straight into it: one
// Write is one log record, which may itself span several lines when the message
// carries newlines.
type Buffer struct {
	capacity int

	mu      sync.Mutex
	entries []Entry
	start   int
	size    int
	nextSeq int64
}

// New returns an empty buffer keeping at most capacity lines. A capacity of zero
// or less falls back to DefaultCapacity, so a misconfigured caller still gets a
// usable buffer instead of one that silently swallows everything.
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}

	return &Buffer{capacity: capacity, entries: make([]Entry, capacity)}
}

// std is the buffer Install taps the global logger into and Default hands out.
// It exists before Install so a caller reading it early gets an empty buffer
// rather than a nil one.
var std = New(DefaultCapacity)

// installOnce keeps a second Install from wrapping the tee around itself, which
// would record every line twice.
var installOnce sync.Once

// Default returns the process wide buffer. It is usable - and empty - whether or
// not Install has run.
func Default() *Buffer {
	return std
}

// Install starts capturing the global logger into the default buffer and returns
// it. Whatever the logger wrote to before keeps receiving every line: this adds
// a reader, it does not move the stream.
//
// It is idempotent, and it is meant to run once at startup, before anything
// worth reading is logged.
func Install() *Buffer {
	installOnce.Do(func() {
		previous := logger.Writer()

		// the buffer goes first: io.MultiWriter stops at the first error, and a
		// desktop app launched from Finder may well have a stdout that fails.
		// Capturing must not depend on the console being there.
		logger.SetOutput(io.MultiWriter(std, previous))
	})

	return std
}

// Write captures one log record. It never fails and never blocks on anything but
// its own lock, because it sits in the path of every log call in the process.
func (b *Buffer) Write(p []byte) (int, error) {
	b.Record(string(p))

	return len(p), nil
}

// Record captures one already rendered log record, splitting it into lines. The
// level is read from the record's own prefix; continuation lines of a multi line
// message inherit it, so a stack trace logged as an error does not turn into a
// wall of info lines.
func (b *Buffer) Record(record string) {
	record = strings.TrimRight(record, "\n")
	if strings.TrimSpace(record) == "" {
		return
	}

	level := LevelInfo

	for i, raw := range strings.Split(record, "\n") {
		text := raw

		if i == 0 {
			parsed := parseLine(raw)
			level, text = parsed.level, parsed.text
		}

		b.append(level, runlog.Truncate(text, MaxLineRunes))
	}
}

// parsedLine is what one raw line said about itself: the level its prefix
// declared and the message left once the prefix is stripped.
type parsedLine struct {
	level string
	text  string
}

// parseLine splits the global logger's prefix off one line. A line that does not
// carry the prefix - anything else writing to the same stream - is kept whole at
// info level rather than being mangled or dropped.
func parseLine(raw string) parsedLine {
	if !hasStamp(raw) {
		return parsedLine{level: LevelInfo, text: raw}
	}

	level, ok := levelOfTag(raw[stampLen+1 : stampLen+1+tagLen])
	if !ok {
		return parsedLine{level: LevelInfo, text: raw}
	}

	return parsedLine{level: level, text: strings.TrimLeft(raw[stampLen+1+tagLen:], " ")}
}

// hasStamp reports whether the line starts with the logger's fixed width
// timestamp followed by a four character tag.
func hasStamp(line string) bool {
	if len(line) < stampLen+1+tagLen {
		return false
	}

	for _, at := range []struct {
		index int
		char  byte
	}{
		{4, '/'}, {7, '/'}, {10, ' '}, {13, ':'}, {16, ':'}, {19, '.'}, {stampLen, ' '},
	} {
		if line[at.index] != at.char {
			return false
		}
	}

	return true
}

// levelOfTag maps a global logger tag onto a level name, reporting whether the
// tag was one it actually issues.
func levelOfTag(tag string) (string, bool) {
	switch tag {
	case logger.TagTrace, logger.TagDebug:
		return LevelDebug, true
	case logger.TagInfo, logger.TagPrint:
		return LevelInfo, true
	case logger.TagWarn:
		return LevelWarn, true
	case logger.TagError, logger.TagFatal, logger.TagPanic:
		return LevelError, true
	default:
		return "", false
	}
}

// Since returns the newest kept lines with a sequence above afterSeq, oldest
// first. A limit of zero or less returns every line it still has.
//
// The newest lines win when the limit bites: a caller watching a running push
// wants the end of the log, and Dropped tells it how much of the middle it lost.
func (b *Buffer) Since(afterSeq int64, limit int) Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	snapshot := Snapshot{Capacity: b.capacity, LatestSeq: b.nextSeq}

	wanted := b.nextSeq - afterSeq
	if wanted <= 0 {
		return snapshot
	}

	// lines the ring already overwrote are gone for good; say so instead of
	// handing back a tail that silently starts later than the caller asked.
	if wanted > int64(b.size) {
		snapshot.Dropped = wanted - int64(b.size)
		wanted = int64(b.size)
	}

	if limit > 0 && wanted > int64(limit) {
		snapshot.Dropped += wanted - int64(limit)
		wanted = int64(limit)
	}

	count := int(wanted)
	snapshot.Entries = make([]Entry, 0, count)

	for i := b.size - count; i < b.size; i++ {
		snapshot.Entries = append(snapshot.Entries, b.entries[(b.start+i)%b.capacity])
	}

	return snapshot
}

// Clear forgets every kept line. Sequences keep counting up, so a page that
// cleared the view and polls on with its old cursor sees the lines logged after
// the clear and nothing else.
func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = make([]Entry, b.capacity)
	b.start = 0
	b.size = 0
}

// Len returns how many lines the buffer currently keeps.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.size
}

// Capacity returns how many lines the buffer keeps at most.
func (b *Buffer) Capacity() int {
	return b.capacity
}

// append stores one line, overwriting the oldest once the ring is full.
func (b *Buffer) append(level, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextSeq++

	entry := Entry{
		Seq:   b.nextSeq,
		Time:  time.Now().UnixMilli(),
		Level: level,
		Text:  text,
	}

	if b.size < b.capacity {
		b.entries[(b.start+b.size)%b.capacity] = entry
		b.size++

		return
	}

	b.entries[b.start] = entry
	b.start = (b.start + 1) % b.capacity
}
