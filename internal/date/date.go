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

package date

import (
	"fmt"
	"time"
)

func GetDateInfo() string {
	now := time.Now()
	weekday := now.Weekday()

	dateString := fmt.Sprintf("今天是 %s %s\n\n", now.Format("2006-01-02"), weekday.String())

	if weekday == time.Sunday || weekday == time.Saturday {
		dateString += "\n周末愉快!"
	}

	return dateString
}
