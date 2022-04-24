package feed

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatURL(t *testing.T) {
	assert.Equal(t, "https://www.baidu.com/abc", FormatURL("https://www.baidu.com/abc?utm_source=aaa"))
	assert.Equal(t, "https://www.baidu.com/abc?p=1", FormatURL("https://www.baidu.com/abc?utm_source=aaa&p=1"))
}
