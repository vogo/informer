package util_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wongoo/informer/internal/util"
)

func TestGetMatchRender(t *testing.T) {
	t.Parallel()

	data := `id:123,title:abc; id:456,title:def; id:789,title:ghi`

	re, _ := regexp.Compile("id:([^,]+),title:([^;]+)")
	match := re.FindAllSubmatch([]byte(data), -1)

	render := util.RegexMatchRender("https://www.example.com/$1?title=$2")

	assert.Equal(t, 3, len(match))
	assert.Equal(t, []byte("https://www.example.com/123?title=abc"), render(match[0]))
	assert.Equal(t, []byte("https://www.example.com/456?title=def"), render(match[1]))
	assert.Equal(t, []byte("https://www.example.com/789?title=ghi"), render(match[2]))
}
