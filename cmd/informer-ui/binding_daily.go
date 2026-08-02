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

// DailyDayDTO is one stored daily report in the date list.
type DailyDayDTO struct {
	// Date is the 2006-01-02 key DailyContent takes.
	Date string `json:"date"`

	// Size is the byte size of the markdown file, shown as a hint for empty days.
	Size int64 `json:"size"`
}

// DailyMonthDTO groups the days of one month, newest first.
type DailyMonthDTO struct {
	Month string         `json:"month"`
	Days  []*DailyDayDTO `json:"days"`
}

// DailyYearDTO groups the months of one year, newest first.
type DailyYearDTO struct {
	Year   string           `json:"year"`
	Months []*DailyMonthDTO `json:"months"`
}

// DailyIndex returns every stored daily report grouped by year and month.
//
// Only file names are read, so opening the tab stays cheap no matter how many years
// of reports the data directory holds; the markdown itself is loaded one day at a
// time by DailyContent.
func (a *App) DailyIndex() ([]*DailyYearDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	years, err := a.svc.DailyIndex()
	if err != nil {
		return nil, err
	}

	dtos := make([]*DailyYearDTO, 0, len(years))

	for _, year := range years {
		months := make([]*DailyMonthDTO, 0, len(year.Months))

		for _, month := range year.Months {
			days := make([]*DailyDayDTO, 0, len(month.Days))
			for _, day := range month.Days {
				days = append(days, &DailyDayDTO{Date: day.Date, Size: day.Size})
			}

			months = append(months, &DailyMonthDTO{Month: month.Month, Days: days})
		}

		dtos = append(dtos, &DailyYearDTO{Year: year.Year, Months: months})
	}

	return dtos, nil
}

// DailyContent returns the markdown of one day. The service validates the date, so a
// crafted value can never reach outside the data directory.
func (a *App) DailyContent(date string) (string, error) {
	err := a.ready()
	if err != nil {
		return "", err
	}

	return a.svc.DailyContent(date)
}
