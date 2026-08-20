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

// Package diagnose repairs a subscription whose parse stopped working.
//
// A regex source breaks when the page it reads changes shape, and the fix is
// always the same shallow edit - a different capture group, a moved path, a feed
// that now lives at another address. What makes it tedious is that finding the
// edit means looking at the bytes informer actually received and trying a
// candidate against them, over and over.
//
// This package hands exactly that loop to an agent, and nothing else. It offers
// three tools - read the configuration, read the fetched document, try a
// candidate configuration - and it deliberately offers no way to save. The agent
// answers with a proposed change; informer re-verifies it on its own and a person
// decides whether it is applied. An agent that could write to the database would
// be one bad regex away from turning a source that half worked into one that
// does not work at all, with the original already overwritten.
//
// What a candidate configuration is made of, and how one is tried out, lives in
// internal/parsecfg: composing a new subscription needs the same vocabulary and
// the same trial, and two copies of it would be two dialects within a week.
package diagnose

import "github.com/vogo/informer/internal/parsecfg"

// Changes is the set of parse affecting fields a diagnosis proposes to edit.
type Changes = parsecfg.Changes

// FieldChange is one field a diagnosis proposes to edit, in the shape a person
// reads it: what it is now and what it would become.
type FieldChange = parsecfg.FieldChange

// RepairableFields lists the columns a diagnosis may edit, in declaration order.
// The prompt states this list, so what the agent is allowed to touch and what it
// is told about can never drift apart.
func RepairableFields() []string {
	return parsecfg.Fields()
}
