package parser

// unsupportedFormats lists packet type characters that aprsgo does not attempt
// to decode into structured data. They are still accepted as raw packets by the
// server layer (which only needs From/To/Path); the parser records them as
// "invalid" so the caller can decide what to do.
//
// Types that used to live here but are now handled (item ')', query '?',
// NMEA '$', telemetry 'T') have been removed.
var unsupportedFormats = map[string]string{
	"%":  "agrelo dfjr",
	"&":  "reserved",
	"(":  "unused",
	"+":  "reserved",
	"-":  "unused",
	".":  "reserved",
	"<":  "station capabilities",
	"[":  "maidenhead locator beacon",
	"\\": "unused",
	"]":  "unused",
	"^":  "unused",
}
