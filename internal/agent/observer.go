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

package agent

import "fmt"

// Note levels. They are plain strings rather than a type from a logging package
// on purpose: this package drives a subprocess and parses json, and staying free
// of the rest of informer is what keeps it testable on its own.
const (
	NoteInfo  = "info"
	NoteWarn  = "warn"
	NoteError = "error"
)

// Observer watches one agent run as it happens.
//
// A browsing agent works for minutes behind a single blocking call, so without
// this the caller has nothing to show for the wait. Note is called from more
// than one goroutine - the answer stream and the error stream are read at the
// same time - so an implementation has to guard itself.
//
// A nil Observer is legal everywhere and means the run is not watched.
type Observer interface {
	Note(level, text string)
}

// ObserverFunc adapts a plain function to an Observer.
type ObserverFunc func(level, text string)

// Note passes the note to the wrapped function, and does nothing when there is
// none.
func (f ObserverFunc) Note(level, text string) {
	if f == nil {
		return
	}

	f(level, text)
}

// notef renders one note for an observer that may not be there.
func notef(observer Observer, level, format string, a ...any) {
	if observer == nil {
		return
	}

	observer.Note(level, fmt.Sprintf(format, a...))
}
