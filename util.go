package termix

import (
	"net/url"
	"strconv"
)

func cloneValues(q url.Values) url.Values {
	n := url.Values{}
	for k, v := range q {
		n[k] = append([]string{}, v...)
	}
	return n
}

func itoa(i int) string { return strconv.Itoa(i) }

func firstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
