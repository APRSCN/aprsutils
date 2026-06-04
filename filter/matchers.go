package filter

import (
	"strconv"
	"strings"

	"github.com/APRSCN/aprsutils"
	"github.com/APRSCN/aprsutils/parser"
)

// rangeData holds precompiled center/distance for r/, m/, f/ filters.
type rangeData struct {
	lat, lon float64
	dist     float64 // km
	call     string  // for f/
}

// areaData holds a precompiled bounding box for a/.
type areaData struct {
	latN, lonW, latS, lonE float64
}

// typeData holds precompiled type bits and optional range for t/.
type typeData struct {
	bits       parser.PacketType
	hasRange   bool
	call       string
	dist       float64
	allButCWOP bool
	wantWX     bool // 'w': regular weather (excluding CWOP)
	wantCWOP   bool // 'c': CWOP weather only
}

// symbolData holds precompiled symbol matching sets for s/.
type symbolData struct {
	primary   string // chars on primary table '/'
	alternate string // chars on alternate table '\'
	overlay   string // overlay characters
}

// qData holds precompiled q-construct matching for q/.
type qData struct {
	cons      string // accepted third chars after "qA", e.g. "CXU"
	analytics bool   // q//i : packets from known igates
}

// precompile fills sp.compiled with type-specific parsed data and validates
// argument counts. It returns false to reject malformed specs.
func precompile(sp *spec) bool {
	switch sp.typ {
	case 'r', 'm', 'f':
		return precompileRange(sp)
	case 'a':
		return precompileArea(sp)
	case 't':
		return precompileType(sp)
	case 's':
		return precompileSymbol(sp)
	case 'q':
		return precompileQ(sp)
	default:
		// b/d/e/g/o/O/p/u just need a non-empty arg list.
		return len(sp.args) > 0
	}
}

func precompileRange(sp *spec) bool {
	rd := &rangeData{}
	switch sp.typ {
	case 'r': // r/lat/lon/dist
		if len(sp.args) < 3 {
			return false
		}
		var err1, err2, err3 error
		rd.lat, err1 = parseFloat(sp.args[0])
		rd.lon, err2 = parseFloat(sp.args[1])
		rd.dist, err3 = parseFloat(sp.args[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return false
		}
	case 'm': // m/dist
		if len(sp.args) < 1 {
			return false
		}
		var err error
		rd.dist, err = parseFloat(sp.args[0])
		if err != nil {
			return false
		}
	case 'f': // f/call/dist
		if len(sp.args) < 2 {
			return false
		}
		rd.call = strings.ToUpper(sp.args[0])
		var err error
		rd.dist, err = parseFloat(sp.args[1])
		if err != nil {
			return false
		}
	}
	if rd.dist < 0 {
		return false
	}
	sp.compiled = rd
	return true
}

func precompileArea(sp *spec) bool {
	if len(sp.args) < 4 {
		return false
	}
	ad := &areaData{}
	var e [4]error
	ad.latN, e[0] = parseFloat(sp.args[0])
	ad.lonW, e[1] = parseFloat(sp.args[1])
	ad.latS, e[2] = parseFloat(sp.args[2])
	ad.lonE, e[3] = parseFloat(sp.args[3])
	for _, err := range e {
		if err != nil {
			return false
		}
	}
	// Require latN >= latS and lonW <= lonE.
	if ad.latN < ad.latS || ad.lonW > ad.lonE {
		return false
	}
	sp.compiled = ad
	return true
}

func precompileType(sp *spec) bool {
	if len(sp.args) < 1 || sp.args[0] == "" {
		return false
	}
	td := &typeData{}
	for _, c := range sp.args[0] {
		switch c {
		case 'p':
			td.bits |= parser.TypePosition
		case 'o':
			td.bits |= parser.TypeObject
		case 'i':
			td.bits |= parser.TypeItem
		case 'm':
			td.bits |= parser.TypeMessage
		case 'q':
			td.bits |= parser.TypeQuery
		case 's':
			td.bits |= parser.TypeStatus
		case 't':
			td.bits |= parser.TypeTelemetry
		case 'u':
			td.bits |= parser.TypeUserDef
		case 'w':
			// Regular weather: matches weather packets EXCEPT CWOP.
			td.wantWX = true
		case 'n':
			td.bits |= parser.TypeNWS
		case 'c':
			// CWOP weather only.
			td.wantCWOP = true
		case '*':
			td.allButCWOP = true
		default:
			// Unknown type letter: ignore.
		}
	}
	// Optional range extension: t/types/call/km
	if len(sp.args) >= 3 {
		td.call = strings.ToUpper(sp.args[1])
		d, err := parseFloat(sp.args[2])
		if err == nil && d >= 0 {
			td.dist = d
			td.hasRange = true
		}
	}
	sp.compiled = td
	return true
}

func precompileSymbol(sp *spec) bool {
	// s/primary[/alternate[/overlay]]
	sd := &symbolData{}
	if len(sp.args) >= 1 {
		sd.primary = sp.args[0]
	}
	if len(sp.args) >= 2 {
		sd.alternate = sp.args[1]
	}
	if len(sp.args) >= 3 {
		sd.overlay = sp.args[2]
	}
	if sd.primary == "" && sd.alternate == "" && sd.overlay == "" {
		return false
	}
	sp.compiled = sd
	return true
}

func precompileQ(sp *spec) bool {
	qd := &qData{}
	if len(sp.args) >= 1 {
		qd.cons = sp.args[0]
	}
	if len(sp.args) >= 2 && (sp.args[1] == "i" || sp.args[1] == "I") {
		qd.analytics = true
	}
	if qd.cons == "" && !qd.analytics {
		return false
	}
	sp.compiled = qd
	return true
}

// --- matchers -------------------------------------------------------------

func matchRange(sp *spec, pkt *parser.Parsed, _ Context) bool {
	if !pkt.HasPosition {
		return false
	}
	rd := sp.compiled.(*rangeData)
	d := aprsutils.CalculateDistanceHaversine(rd.lat, rd.lon, pkt.Lat, pkt.Lon)
	return d <= rd.dist
}

func matchArea(sp *spec, pkt *parser.Parsed, _ Context) bool {
	if !pkt.HasPosition {
		return false
	}
	ad := sp.compiled.(*areaData)
	return pkt.Lat <= ad.latN && pkt.Lat >= ad.latS &&
		pkt.Lon >= ad.lonW && pkt.Lon <= ad.lonE
}

func matchMyRange(sp *spec, pkt *parser.Parsed, ctx Context) bool {
	if !pkt.HasPosition {
		return false
	}
	pos, ok := ctx.ClientPosition()
	if !ok {
		return false
	}
	rd := sp.compiled.(*rangeData)
	d := aprsutils.CalculateDistanceHaversine(pos.Lat, pos.Lon, pkt.Lat, pkt.Lon)
	return d <= rd.dist
}

func matchFriend(sp *spec, pkt *parser.Parsed, ctx Context) bool {
	if !pkt.HasPosition {
		return false
	}
	rd := sp.compiled.(*rangeData)
	pos, ok := ctx.StationPosition(rd.call)
	if !ok {
		return false
	}
	d := aprsutils.CalculateDistanceHaversine(pos.Lat, pos.Lon, pkt.Lat, pkt.Lon)
	return d <= rd.dist
}

func matchPrefix(sp *spec, pkt *parser.Parsed, _ Context) bool {
	src := strings.ToUpper(srcCall(pkt))
	for _, pfx := range sp.args {
		if pfx == "" {
			continue
		}
		if strings.HasPrefix(src, strings.ToUpper(pfx)) {
			return true
		}
	}
	return false
}

func matchBuddy(sp *spec, pkt *parser.Parsed, _ Context) bool {
	src := srcCall(pkt)
	for _, pat := range sp.args {
		if matchWild(pat, src) {
			return true
		}
	}
	return false
}

func matchObject(sp *spec, pkt *parser.Parsed, _ Context) bool {
	if !pkt.PacketType.Has(parser.TypeObject | parser.TypeItem) {
		return false
	}
	name := pkt.ObjectName
	if name == "" {
		return false
	}
	for _, pat := range sp.args {
		pat = strings.ReplaceAll(pat, "|", "/")
		pat = strings.ReplaceAll(pat, "~", "*")
		if matchWild(pat, name) {
			return true
		}
	}
	return false
}

func matchType(sp *spec, pkt *parser.Parsed, ctx Context) bool {
	td := sp.compiled.(*typeData)

	isCWOP := pkt.PacketType.Has(parser.TypeCWOP)
	isWX := pkt.PacketType.Has(parser.TypeWeather)

	matched := pkt.PacketType.Has(td.bits)
	// 'w' matches weather that is NOT CWOP.
	if td.wantWX && isWX && !isCWOP {
		matched = true
	}
	// 'c' matches CWOP weather only.
	if td.wantCWOP && isCWOP {
		matched = true
	}
	// '*' matches everything except CWOP.
	if td.allButCWOP && !isCWOP {
		matched = true
	}
	if !matched {
		return false
	}
	if !td.hasRange {
		return true
	}
	// Ranged type filter: packet must also be within dist km of the named
	// station's last-known position.
	if !pkt.HasPosition {
		return false
	}
	pos, ok := ctx.StationPosition(td.call)
	if !ok {
		return false
	}
	d := aprsutils.CalculateDistanceHaversine(pos.Lat, pos.Lon, pkt.Lat, pkt.Lon)
	return d <= td.dist
}

func matchSymbol(sp *spec, pkt *parser.Parsed, _ Context) bool {
	if len(pkt.Symbol) < 2 {
		return false
	}
	sd := sp.compiled.(*symbolData)
	// parser.Symbol is [symbolCode, symbolTable].
	code := pkt.Symbol[0]
	table := pkt.Symbol[1]

	switch table {
	case "/":
		return strings.Contains(sd.primary, code)
	case "\\":
		if sd.overlay == "" {
			return strings.Contains(sd.alternate, code)
		}
		return strings.Contains(sd.alternate, code)
	default:
		// Overlaid symbol: table char is the overlay (not '/' or '\').
		if sd.overlay != "" && strings.Contains(sd.overlay, table) {
			return strings.Contains(sd.alternate, code)
		}
		if sd.alternate != "" {
			return strings.Contains(sd.alternate, code)
		}
		return false
	}
}

func matchDigi(sp *spec, pkt *parser.Parsed, _ Context) bool {
	// Match callsigns that have been used (have a trailing '*') in the path,
	// up to the q-construct.
	for _, hop := range pktPathBeforeQ(pkt) {
		used := strings.HasSuffix(hop, "*")
		call := strings.TrimSuffix(hop, "*")
		if !used {
			continue
		}
		for _, pat := range sp.args {
			if matchWild(pat, call) {
				return true
			}
		}
	}
	return false
}

func matchEntry(sp *spec, pkt *parser.Parsed, _ Context) bool {
	entry := entryCall(pkt)
	if entry == "" {
		return false
	}
	for _, pat := range sp.args {
		if matchWild(pat, entry) {
			return true
		}
	}
	return false
}

func matchGroup(sp *spec, pkt *parser.Parsed, _ Context) bool {
	if !pkt.PacketType.Has(parser.TypeMessage) {
		return false
	}
	if pkt.Addressee == "" {
		return false
	}
	for _, pat := range sp.args {
		if matchWild(pat, pkt.Addressee) {
			return true
		}
	}
	return false
}

func matchUnproto(sp *spec, pkt *parser.Parsed, _ Context) bool {
	for _, pat := range sp.args {
		if matchWild(pat, pkt.To) {
			return true
		}
	}
	return false
}

func matchQConstruct(sp *spec, pkt *parser.Parsed, _ Context) bool {
	qd := sp.compiled.(*qData)
	qChar, hasQ := qConstructChar(pkt)

	if qd.cons != "" && hasQ {
		if strings.IndexByte(qd.cons, qChar) >= 0 {
			return true
		}
	}
	if qd.analytics {
		// Packets relayed by an igate: qAR/qAr/qAo/qAO.
		if hasQ && (qChar == 'R' || qChar == 'r' || qChar == 'o' || qChar == 'O') {
			return true
		}
	}
	return false
}

// --- helpers --------------------------------------------------------------

// parseFloat parses a float, accepting an optional leading/trailing space.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// srcCall returns the effective source callsign, descending into third-party
// traffic so b/ and p/ match the inner source.
func srcCall(pkt *parser.Parsed) string {
	if pkt.PacketType.Has(parser.TypeThirdParty) && pkt.SubPacket != nil && pkt.SubPacket.From != "" {
		return pkt.SubPacket.From
	}
	return pkt.From
}

// qConstructChar returns the third character of the q-construct (e.g. 'C' for
// qAC) and whether one is present in the path.
func qConstructChar(pkt *parser.Parsed) (byte, bool) {
	for _, hop := range pkt.Path {
		if len(hop) == 3 && (hop[0] == 'q') && hop[1] == 'A' {
			return hop[2], true
		}
	}
	return 0, false
}

// pktPathBeforeQ returns path hops up to (excluding) the q-construct.
func pktPathBeforeQ(pkt *parser.Parsed) []string {
	out := make([]string, 0, len(pkt.Path))
	for _, hop := range pkt.Path {
		if len(hop) == 3 && hop[0] == 'q' && hop[1] == 'A' {
			break
		}
		out = append(out, hop)
	}
	return out
}

// entryCall returns the entry station callsign: the path element immediately
// following the q-construct (the igate/server that injected the packet).
func entryCall(pkt *parser.Parsed) string {
	for i, hop := range pkt.Path {
		if len(hop) == 3 && hop[0] == 'q' && hop[1] == 'A' {
			if i+1 < len(pkt.Path) {
				return strings.TrimSuffix(pkt.Path[i+1], "*")
			}
			return ""
		}
	}
	return ""
}

// matchWild performs case-insensitive matching with '*' as a trailing/embedded
// wildcard. A pattern of exactly "*" matches anything. Without a '*', the match
// is exact (this is what distinguishes b/ from p/).
func matchWild(pattern, text string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	p := strings.ToUpper(pattern)
	t := strings.ToUpper(text)

	if !strings.Contains(p, "*") {
		return p == t
	}

	// Split on '*' and match the fixed segments in order, anchoring the first
	// segment to the start and the last to the end (unless adjacent to '*').
	parts := strings.Split(p, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			if !strings.HasPrefix(t[pos:], part) {
				return false
			}
			pos += len(part)
			continue
		}
		idx := strings.Index(t[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	// If the pattern does not end with '*', the last segment must reach the end.
	if !strings.HasSuffix(p, "*") {
		last := parts[len(parts)-1]
		if last != "" && !strings.HasSuffix(t, last) {
			return false
		}
	}
	return true
}
