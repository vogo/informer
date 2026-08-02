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

// Package configstore reads and writes the human editable json configuration files
// of informer.
//
// The configuration stays a plain file that a user may open in an editor, so the
// package is built around three promises:
//
//   - a save keeps every top level field the file already had, including fields this
//     build does not know about, and keeps them in their original order;
//   - a save replaces the file atomically, so a reader - a crontab run for example -
//     never observes a half written document;
//   - concurrent writers serialize through an advisory lock file with a timeout, so a
//     second writer either waits its turn or fails loudly, and never deadlocks.
package configstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Doc is one json object read from disk with its field order preserved.
// A field this build does not model is carried through a save untouched.
type Doc struct {
	// order lists the top level keys in the order the file spelled them.
	order []string

	// fields holds the raw value of every top level key.
	fields map[string]json.RawMessage

	// exists reports whether the document was actually read from a file.
	exists bool
}

// NewDoc returns an empty document, the state a missing configuration file has.
func NewDoc() *Doc {
	return &Doc{fields: map[string]json.RawMessage{}}
}

// Exists reports whether the document came from an existing file.
func (d *Doc) Exists() bool {
	return d.exists
}

// Keys returns the top level keys in file order.
func (d *Doc) Keys() []string {
	keys := make([]string, len(d.order))
	copy(keys, d.order)

	return keys
}

// Unmarshal decodes the value of one key into target.
// It reports false when the key is absent, leaving target untouched.
func (d *Doc) Unmarshal(key string, target any) (bool, error) {
	raw, ok := d.fields[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}

	err := json.Unmarshal(raw, target)
	if err != nil {
		return false, fmt.Errorf("parse config field %q: %w", key, err)
	}

	return true, nil
}

// Set replaces the value of one key, appending it at the end when it is new.
func (d *Doc) Set(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode config field %q: %w", key, err)
	}

	if _, ok := d.fields[key]; !ok {
		d.order = append(d.order, key)
	}

	d.fields[key] = raw

	return nil
}

// Bytes renders the document as indented json, in the original field order and with
// a trailing newline, so the result stays a file a human can read and diff.
func (d *Doc) Bytes() ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteString("{\n")

	for i, key := range d.order {
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, fmt.Errorf("encode config key %q: %w", key, err)
		}

		indented := &bytes.Buffer{}

		err = json.Indent(indented, d.fields[key], "  ", "  ")
		if err != nil {
			return nil, fmt.Errorf("format config field %q: %w", key, err)
		}

		buf.WriteString("  ")
		buf.Write(encodedKey)
		buf.WriteString(": ")
		buf.Write(indented.Bytes())

		if i < len(d.order)-1 {
			buf.WriteByte(',')
		}

		buf.WriteByte('\n')
	}

	buf.WriteString("}\n")

	return buf.Bytes(), nil
}

// Load reads the json object stored at path.
// A missing file is not an error: it yields an empty document, so the first save of a
// fresh installation writes a complete file instead of failing on the read.
func Load(path string) (*Doc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewDoc(), nil
		}

		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	doc, err := parseDoc(raw)
	if err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	doc.exists = true

	return doc, nil
}

// parseDoc decodes a json object while remembering the order of its keys.
func parseDoc(raw []byte) (*Doc, error) {
	doc := NewDoc()

	if len(bytes.TrimSpace(raw)) == 0 {
		return doc, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))

	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read opening token: %w", err)
	}

	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("%w: the configuration must be a json object", ErrInvalidConfig)
	}

	for decoder.More() {
		keyToken, keyErr := decoder.Token()
		if keyErr != nil {
			return nil, fmt.Errorf("read field name: %w", keyErr)
		}

		key, isString := keyToken.(string)
		if !isString {
			return nil, fmt.Errorf("%w: field name %v is not a string", ErrInvalidConfig, keyToken)
		}

		var value json.RawMessage

		valueErr := decoder.Decode(&value)
		if valueErr != nil {
			return nil, fmt.Errorf("read value of field %q: %w", key, valueErr)
		}

		err = doc.Set(key, value)
		if err != nil {
			return nil, err
		}
	}

	_, err = decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read closing token: %w", err)
	}

	// anything after the object means the file is not the single configuration
	// document informer writes, and a save would silently drop it.
	_, err = decoder.Token()
	if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content after the json object", ErrInvalidConfig)
	}

	return doc, nil
}
