package util

import "strconv"

func RegexMatchRender(renderExpression string) func([][]byte) []byte {
	start := 0
	var list []func([][]byte) []byte

	length := len(renderExpression)
	for i := 0; i < length; i++ {
		v := renderExpression[i]

		if v == '$' {
			if i > start {
				sub := []byte(renderExpression[start:i])
				list = append(list, func([][]byte) []byte {
					return sub
				})
			}

			j := i + 1
			for ; j < length && (renderExpression[j] >= '0' && renderExpression[j] <= '9'); j++ {
			}

			group, _ := strconv.Atoi(renderExpression[i+1 : j])
			list = append(list, func(groups [][]byte) []byte {
				return groups[group]
			})

			start = j
		}
	}

	if start < length {
		sub := []byte(renderExpression[start:length])
		list = append(list, func([][]byte) []byte {
			return sub
		})
	}

	return func(groups [][]byte) []byte {
		var buf []byte

		for _, f := range list {
			buf = append(buf, f(groups)...)
		}

		return buf
	}
}
