// Package filter implements the standard APRS-IS server-side packet filters
// (14 types):
//
//	a/  area (bounding box)        b/  buddy (source callsign, wildcards)
//	d/  used digipeater            e/  entry station (q-construct igate)
//	f/  friend range               g/  message recipient
//	m/  my range                   o/  object/item name
//	p/  source prefix              q/  q-construct
//	r/  range (lat/lon/dist)       s/  symbol
//	t/  type (+ optional range)    u/  unproto (destination)
//
// A filter string is a space-separated list of specs. A leading '-' negates a
// spec (matching packets are dropped). Type letters and the q construct value
// are case-sensitive; callsign/prefix matching is not.
//
// Stateful filters (m/, f/ and the ranged form of t/) need station positions,
// supplied by the caller through a Context.
package filter

import (
	"strings"

	"github.com/APRSCN/aprsutils/parser"
)

// Position is a geographic coordinate in decimal degrees.
type Position struct {
	Lat float64
	Lon float64
}

// Context supplies stateful information that some filters need.
//
// Implementations are provided by the server layer (e.g. backed by a
// historydb of last-known station positions). A nil Context is acceptable;
// stateful filters that need data the Context cannot provide simply do not
// match.
type Context interface {
	// ClientPosition returns the requesting client's own last-known position.
	// ok is false when the position is unknown (m/ then cannot match).
	ClientPosition() (pos Position, ok bool)
	// StationPosition returns the last-known position of an arbitrary station
	// by callsign. ok is false when unknown.
	StationPosition(callsign string) (pos Position, ok bool)
}

// nilContext is used when the caller passes a nil Context.
type nilContext struct{}

func (nilContext) ClientPosition() (Position, bool)        { return Position{}, false }
func (nilContext) StationPosition(string) (Position, bool) { return Position{}, false }

// Filter is a compiled, reusable set of filter specs.
type Filter struct {
	specs []spec
	raw   string
}

// spec is a single compiled filter clause.
type spec struct {
	typ      byte     // filter type letter ('r', 'p', ...)
	negate   bool     // true if prefixed with '-'
	args     []string // raw '/'-separated arguments after the type
	raw      string   // original text of this spec (for diagnostics)
	matcher  matchFunc
	compiled any // type-specific precompiled data
}

// matchFunc evaluates a single spec against a packet.
type matchFunc func(s *spec, pkt *parser.Parsed, ctx Context) bool

// Compile parses a filter string into a reusable Filter. Unknown or malformed
// specs are skipped but recorded in the raw string. It never returns an error;
// an empty or whitespace-only string yields a Filter that matches nothing.
func Compile(s string) *Filter {
	f := &Filter{raw: s}
	for _, tok := range strings.Fields(s) {
		sp, ok := compileSpec(tok)
		if ok {
			f.specs = append(f.specs, sp)
		}
	}
	return f
}

// compileSpec compiles a single token such as "r/60/25/100" or "-t/m".
func compileSpec(tok string) (spec, bool) {
	sp := spec{raw: tok}
	if tok == "" {
		return sp, false
	}
	if tok[0] == '-' {
		sp.negate = true
		tok = tok[1:]
	}
	if len(tok) < 2 {
		return sp, false
	}

	// The type is the leading letters up to the first '/'. Most types are a
	// single letter; 'os' (strict object) is the only two-letter type.
	slash := strings.IndexByte(tok, '/')
	var head string
	if slash < 0 {
		head = tok
	} else {
		head = tok[:slash]
		sp.args = strings.Split(tok[slash+1:], "/")
	}

	switch head {
	case "os":
		sp.typ = 'O' // strict object, distinguished from 'o'
		sp.matcher = matchObject
	default:
		sp.typ = head[0]
		m, ok := matchers[sp.typ]
		if !ok {
			return sp, false
		}
		sp.matcher = m
	}

	if !precompile(&sp) {
		return sp, false
	}
	return sp, true
}

// matchers maps a type letter to its matching function.
var matchers = map[byte]matchFunc{
	'a': matchArea,
	'b': matchBuddy,
	'd': matchDigi,
	'e': matchEntry,
	'f': matchFriend,
	'g': matchGroup,
	'm': matchMyRange,
	'o': matchObject,
	'p': matchPrefix,
	'q': matchQConstruct,
	'r': matchRange,
	's': matchSymbol,
	't': matchType,
	'u': matchUnproto,
}

// Match reports whether the packet should be forwarded to the client owning
// this filter. ctx may be nil. Negated specs are checked first: if any matches,
// the packet is dropped. Otherwise, a positive match passes; with no positive
// specs nothing passes.
func (f *Filter) Match(pkt *parser.Parsed, ctx Context) bool {
	if f == nil || len(f.specs) == 0 {
		return false
	}
	if ctx == nil {
		ctx = nilContext{}
	}

	// First pass: negations take precedence.
	for i := range f.specs {
		sp := &f.specs[i]
		if sp.negate && sp.matcher(sp, pkt, ctx) {
			return false
		}
	}

	// Second pass: any positive match passes.
	for i := range f.specs {
		sp := &f.specs[i]
		if !sp.negate && sp.matcher(sp, pkt, ctx) {
			return true
		}
	}
	return false
}

// String returns the original filter text.
func (f *Filter) String() string {
	if f == nil {
		return ""
	}
	return f.raw
}
