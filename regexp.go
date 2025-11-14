package aprsutils

import (
	"sync"

	regexp "github.com/wasilibs/go-re2"
)

// compiledRegexp is the basic struct to save compiled regexp to accelerator
type compiledRegexp struct {
	l *sync.RWMutex
	r map[string]*regexp.Regexp
}

// CompiledRegexps saves all compiled regexp here
var CompiledRegexps *compiledRegexp

// init the CompiledRegexps var
func init() {
	CompiledRegexps = create()
}

// create a compiledRegexp
func create() *compiledRegexp {
	c := &compiledRegexp{
		l: new(sync.RWMutex),
		r: make(map[string]*regexp.Regexp),
	}
	return c
}

// Get a regexp
func (c *compiledRegexp) Get(expr string) *regexp.Regexp {
	c.l.Lock()
	defer c.l.Unlock()

	// Try to get existed regexp
	if re, ok := c.r[expr]; ok {
		return re
	}

	// Compile a new one
	c.r[expr] = regexp.MustCompile(expr)
	return c.r[expr]
}
