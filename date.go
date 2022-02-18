package informer

import (
	"fmt"
	"time"
)

func getDateInfo() string {
	now := time.Now()
	weekday := now.Weekday()

	dateString := fmt.Sprintf("今天是 %s %s\n\n", now.Format("2006-01-02"), weekday.String())

	if weekday == time.Sunday || weekday == time.Saturday {
		dateString += "\n周末愉快!"
	}

	return dateString
}
