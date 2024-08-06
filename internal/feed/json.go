package feed

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/vogo/logger"
	"github.com/vogo/vogo/vnet/vurl"
)

func JsonParseFeed(config *Config, source *Source, expireTime int64) ([]*Article, error) {
	data, err := readURLData(source)
	if err != nil {
		return nil, err
	}

	hostPrefix := GetHostPrefix(source.URL)

	var jsonData map[string]interface{}
	if jsonErr := json.Unmarshal(data, &jsonData); jsonErr != nil {
		logger.Errorf("json unmarshal error! url: %s, error: %v, data: %s", source.URL, jsonErr, data)
		return nil, nil
	}

	titles := getJSONNestedValue(jsonData, source.JsonTitlePath)
	urls := getJSONNestedValue(jsonData, source.JsonURLPath)
	if len(titles) != len(urls) {
		logger.Errorf("json parse error! url: %s, titles: %v, urls: %v", source.URL, titles, urls)
		return nil, nil
	}
	if len(titles) == 0 {
		logger.Warnf("json parse error! url: %s, titles: %v, urls: %v", source.URL, titles, urls)
		return nil, nil
	}
	//nolint:prealloc //ignore this.
	var articles []*Article

	for i, titleValue := range titles {
		if source.MaxFetchNum > 0 && i >= source.MaxFetchNum {
			break
		}

		link := urls[i].(string)
		link = adjustLink(hostPrefix, link)
		title := titleValue.(string)
		logger.Infof("json parse, link: %s, title: %s", link, title)

		if source.Redirect {
			link = vurl.RedirectURL(link)
		}

		article := &Article{
			URL:       link,
			Title:     title,
			Timestamp: time.Now().Unix(),
			Weight:    source.Weight,
		}

		articles = append(articles, article)
	}

	return articles, nil
}

func getJSONNestedValue(data map[string]interface{}, keyPath string) []interface{} {
	keys := strings.Split(keyPath, "/")

	return parseJsonNestedValue(data, keys)
}

func parseJsonNestedValue(data map[string]interface{}, keys []string) []interface{} {
	var values []interface{}

	keyName := keys[0]
	key := keyName
	isArr := strings.HasSuffix(keyName, "[]")
	if isArr {
		key = keyName[:len(keyName)-2]
	}

	switch v := data[key].(type) {
	case map[string]interface{}:
		if len(keys) == 1 {
			logger.Warnf("leaf value is map! keys:%s, value: %v", keys, v)
		} else {
			values = appendNoneNil(values, parseJsonNestedValue(v, keys[1:]))
		}
	case []interface{}:
		if isArr {
			for _, item := range v {
				if len(keys) == 1 {
					values = append(values, item)
				} else {
					values = appendNoneNil(values, parseJsonNestedValue(item.(map[string]interface{}), keys[1:]))
				}
			}
		} else {
			if len(keys) == 1 {
				values = append(values, v)
			} else {
				values = appendNoneNil(values, parseJsonNestedValue(v[0].(map[string]interface{}), keys[1:]))
			}
		}
	default:
		if len(keys) == 1 {
			values = append(values, v)
		} else {
			logger.Warnf("json nested value not found! keys:%s", keys)
		}
	}

	return values
}

func appendNoneNil(values []interface{}, value []interface{}) []interface{} {
	if len(value) > 0 {
		values = append(values, value...)
	}
	return values
}
