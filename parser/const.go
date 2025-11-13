package parser

var unsupportedFormats = map[string]string{
	"#":  "raw weather report",
	"$":  "raw gps",
	"%":  "agrelo",
	"&":  "reserved",
	"(":  "unused",
	")":  "item report",
	"*":  "complete weather report",
	"+":  "reserved",
	"-":  "unused",
	".":  "reserved",
	"<":  "station capabilities",
	"?":  "general query format",
	"T":  "telemetry report",
	"[":  "maidenhead locator beacon",
	"\\": "unused",
	"]":  "unused",
	"^":  "unused",
}
