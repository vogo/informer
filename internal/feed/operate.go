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

	"github.com/mmcdole/gofeed"
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

	fp := gofeed.NewParser()

	feedData, err := fp.ParseURL(source.URL)
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
	fmt.Printf("weight:\t%d\n", source.Weight)
	fmt.Printf("max_fetch_num:\t%d\n", source.MaxFetchNum)
	fmt.Printf("regex:\t%s\n", source.Regex)
	fmt.Printf("title_exp:\t%s\n", source.TitleExp)
	fmt.Printf("url_exp:\t%s\n", source.URLExp)
	fmt.Printf("title_group:\t%d\n", source.TitleGroup)
	fmt.Printf("url_group:\t%d\n", source.URLGroup)
}

func listSource() {
	var sources []*Source

	feedDataDB.Model(&Source{}).Order("id").Find(&sources)

	for _, source := range sources {
		fmt.Printf("%d,\t%s,\t%s\n", source.ID, source.Title, source.URL)
	}
}
