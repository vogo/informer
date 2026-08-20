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

import "github.com/vogo/informer/internal/runlog"

// PreviewLogEvent is the wails event every test fetch progress line arrives on.
//
// One name carries every run: a listener tells its own lines apart by the run id
// it passed to PreviewSource. A per run event name would grow the runtime's
// listener map on every fetch, and a single missed unsubscribe would leak it.
//
// The frontend has to declare the same name; SourceManager.vue is the other half.
const PreviewLogEvent = "informer:preview:log"

// previewSinkFor returns where one test fetch reports to, or nil for a caller
// that asked for no reporting. A nil sink parses exactly as it always did.
func previewSinkFor(runID string) runlog.Sink {
	return runLogSinkFor(PreviewLogEvent, runID)
}

// PreviewSource runs one real fetch and parse of a stored subscription and
// returns the candidate articles. It writes nothing - not the source, not
// articles, not health state - and a disabled source previews just the same.
//
// runID, when not empty, streams the run's progress to the window on
// PreviewLogEvent, so a fetch that takes minutes - an agent source browses the
// web before it answers - is something the user can watch instead of wait out.
// The id is the caller's own token: the page tells its lines apart by it, which
// is what keeps two test fetches in a row from mixing.
func (a *App) PreviewSource(id int64, runID string) ([]*ArticleDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	articles, err := a.svc.PreviewTraced(id, previewSinkFor(runID))
	if err != nil {
		return nil, err
	}

	dtos := make([]*ArticleDTO, 0, len(articles))
	for _, article := range articles {
		dtos = append(dtos, &ArticleDTO{Title: article.Title, URL: article.URL})
	}

	return dtos, nil
}
