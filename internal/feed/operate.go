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

package feed

import (
	"fmt"
	"strconv"
)

func Operate(ops []string) {
	if len(ops) == 0 {
		return
	}

	op := ops[0]
	switch op {
	case "list":
		listSource()
	case "view":
		viewSource(ops[1])
	case "add":
		AddSource(ops[1], ops[2])
	case "remove":
		removeSource(ops[1])
	case "update":
		updateSource(ops[1], ops[2], ops[3])
	case "parse":
		parseSource(ops[1])
	case "copy":
		copySource(ops[1])
		listSource()
	}
}

func parseSource(idStr string) {
	id, _ := strconv.Atoi(idStr)
	source := &Source{}
	feedDataDB.Model(source).Where("id=?", id).Find(source)

	if source.Regex != "" {
		articles, err := RegexParse(source)
		if err != nil {
			fmt.Println(err)

			return
		}

		for _, item := range articles {
			fmt.Println(item.Title, ":", item.URL)
		}

		return
	}

	feedData, err := ParseFeed(source.URL)
	if err != nil {
		fmt.Println(err)

		return
	}

	for _, item := range feedData.Items {
		fmt.Println(item.Title, ":", item.Link)
	}
}

func AddSource(title, link string) {
	feedDataDB.Create(&Source{
		Title: title,
		URL:   link,
	})
}

func removeSource(id string) {
	sourceID, _ := strconv.Atoi(id)
	feedDataDB.Delete(&Source{}, sourceID)
}

func updateSource(id, column, value string) {
	sourceID, _ := strconv.Atoi(id)
	feedDataDB.Model(&Source{}).Where("id=?", sourceID).Update(column, value)
}

func viewSource(idStr string) {
	id, _ := strconv.Atoi(idStr)
	source := &Source{}
	feedDataDB.Model(source).Where("id=?", id).Find(source)
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
}

func listSource() {
	var sources []*Source

	feedDataDB.Model(&Source{}).Order("id").Find(&sources)

	for _, source := range sources {
		fmt.Printf("%d,\t%s,\t%s\n", source.ID, source.Title, source.URL)
	}
}

func copySource(srcID string) {
	id, _ := strconv.Atoi(srcID)
	source := &Source{}
	feedDataDB.Model(source).Where("id=?", id).Find(source)

	if source.ID == 0 {
		fmt.Println("source not found")

		return
	}

	feedDataDB.Create(&Source{
		Title:       source.Title,
		URL:         source.URL,
		CURL:        source.CURL,
		Weight:      source.Weight,
		MaxFetchNum: source.MaxFetchNum,
		Regex:       source.Regex,
		TitleExp:    source.TitleExp,
		URLExp:      source.URLExp,
		Redirect:    source.Redirect,
	})
}
