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

// Package cli parses command arguments, calls the service layer and formats the
// output. It holds no business logic and never touches the database directly.
package cli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// ErrUsage marks a command invoked with the wrong arguments.
var ErrUsage = errors.New("usage error")

// Feed dispatches the "informer feed ..." commands.
func Feed(svc *service.Service, ops []string) error {
	if len(ops) == 0 {
		return nil
	}

	op := ops[0]
	args := ops[1:]

	switch op {
	case "list":
		return listSource(svc)
	case "view":
		return withOneArg(op, args, func(id string) error { return viewSource(svc, id) })
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("%w: feed add <title> <url>", ErrUsage)
		}

		return addSource(svc, args[0], args[1])
	case "remove":
		return withOneArg(op, args, func(id string) error { return removeSource(svc, id) })
	case "update":
		if len(args) < 3 {
			return fmt.Errorf("%w: feed update <id> <column> <value>", ErrUsage)
		}

		return updateSource(svc, args[0], args[1], args[2])
	case "parse":
		return withOneArg(op, args, func(id string) error { return parseSource(svc, id) })
	case "copy":
		return withOneArg(op, args, func(id string) error { return copySource(svc, id) })
	case "category":
		return listCategory(svc)
	default:
		return fmt.Errorf("%w: unknown feed command %q", ErrUsage, op)
	}
}

func withOneArg(op string, args []string, run func(string) error) error {
	if len(args) < 1 {
		return fmt.Errorf("%w: feed %s <id>", ErrUsage, op)
	}

	return run(args[0])
}

func parseID(idStr string) (int64, error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not an id", ErrUsage, idStr)
	}

	return id, nil
}

func parseSource(svc *service.Service, idStr string) error {
	id, err := parseID(idStr)
	if err != nil {
		return err
	}

	articles, err := svc.Preview(id)
	if err != nil {
		fmt.Println(err)

		return nil
	}

	for _, item := range articles {
		fmt.Println(item.Title, ":", item.URL)
	}

	return nil
}

func addSource(svc *service.Service, title, link string) error {
	source := &feed.Source{Title: title, URL: link}
	if err := svc.CreateSource(source); err != nil {
		return err
	}

	fmt.Printf("%d,\t%s,\t%s\n", source.ID, source.Title, source.URL)

	return nil
}

func removeSource(svc *service.Service, idStr string) error {
	id, err := parseID(idStr)
	if err != nil {
		return err
	}

	return svc.DeleteSource(id)
}

func updateSource(svc *service.Service, idStr, column, value string) error {
	id, err := parseID(idStr)
	if err != nil {
		return err
	}

	return svc.UpdateSourceColumn(id, column, value)
}

func viewSource(svc *service.Service, idStr string) error {
	id, err := parseID(idStr)
	if err != nil {
		return err
	}

	source, err := svc.GetSource(id)
	if err != nil {
		return err
	}

	fmt.Printf("id:\t%d\n", source.ID)
	fmt.Printf("title:\t%s\n", source.Title)
	fmt.Printf("url:\t%s\n", source.URL)
	fmt.Printf("c_url:\t%s\n", source.CURL)
	fmt.Printf("weight:\t%d\n", source.Weight)
	fmt.Printf("max_fetch_num:\t%d\n", source.MaxFetchNum)
	fmt.Printf("regex:\t%s\n", source.Regex)
	fmt.Printf("title_exp:\t%s\n", source.TitleExp)
	fmt.Printf("url_exp:\t%s\n", source.URLExp)
	fmt.Printf("redirect:\t%t\n", source.Redirect)
	fmt.Printf("sort:\t%t\n", source.Sort)
	fmt.Printf("is_json:\t%t\n", source.IsJSON)
	fmt.Printf("json_title_path:\t%s\n", source.JsonTitlePath)
	fmt.Printf("json_url_path:\t%s\n", source.JsonURLPath)
	fmt.Printf("parse_type:\t%s\n", source.ResolveParseType())
	fmt.Printf("category_id:\t%d\n", source.CategoryID)
	fmt.Printf("enabled:\t%t\n", source.Enabled)

	return nil
}

func listSource(svc *service.Service) error {
	sources, err := svc.AllSources(service.SourceQuery{})
	if err != nil {
		return err
	}

	for _, source := range sources {
		fmt.Printf("%d,\t%s,\t%s\n", source.ID, source.Title, source.URL)
	}

	return nil
}

func listCategory(svc *service.Service) error {
	categories, err := svc.AllCategories()
	if err != nil {
		return err
	}

	for _, category := range categories {
		fmt.Printf("%d,\t%s,\t%d\n", category.ID, category.Name, category.Sort)
	}

	return nil
}

func copySource(svc *service.Service, idStr string) error {
	id, err := parseID(idStr)
	if err != nil {
		return err
	}

	source, err := svc.GetSource(id)
	if err != nil {
		fmt.Println("source not found")

		return nil
	}

	duplicate := *source
	duplicate.ID = 0
	duplicate.Status = 0
	duplicate.ErrorInfo = ""

	if err = svc.CreateSource(&duplicate); err != nil {
		return err
	}

	return listSource(svc)
}
