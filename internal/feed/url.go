package feed

import (
	"net/url"
	"strings"

	"github.com/vogo/logger"
)

func FormatURL(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		logger.Error("format url error!", err)
		return link
	}

	params := u.Query()
	for key := range params {
		if strings.HasPrefix(key, "utm_") {
			params.Del(key)
		}
	}

	u.RawQuery = params.Encode()

	return u.String()
}

func IsURLContainsInvalidChars(link string) bool {
	return strings.Contains(link, "%22") || strings.Contains(link, "%20") || strings.Contains(link, "%3C")
}
