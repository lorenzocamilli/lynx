package intercept

import "github.com/lorenzocamilli/lynx/pkg/filter"

type Settings struct {
	RequestsEnabled  bool
	ResponsesEnabled bool
	RequestFilter    filter.Expression
	ResponseFilter   filter.Expression
}
