# aprsutils

[![Go Reference](https://pkg.go.dev/badge/github.com/APRSCN/aprsutils.svg)](https://pkg.go.dev/github.com/APRSCN/aprsutils)

A Go library of building blocks for working with the [APRS-IS](http://www.aprs-is.net/)
network and the [APRS](http://www.aprs.org/) protocol. It provides packet
parsing, server-side filtering, `q`-construct processing, an APRS-IS client,
passcode generation and a handful of related utilities.

It is the algorithm/utility layer behind the
[`aprsgo`](https://github.com/APRSCN/aprsgo) APRS-IS server, but each package is
usable on its own.

## Install

```sh
go get github.com/APRSCN/aprsutils
```

```go
import "github.com/APRSCN/aprsutils"
```

Requires a recent Go toolchain (see [`go.mod`](go.mod) for the exact version).

## Packages

| Import path | Purpose |
|---|---|
| `github.com/APRSCN/aprsutils` | Top-level helpers: passcode, callsign validation, Base91, distance, logger interface. |
| `.../aprsutils/parser` | Parse raw APRS packets into a structured `Parsed` value. |
| `.../aprsutils/filter` | Compile and evaluate APRS-IS server filters (the `a/b/d/e/f/g/m/o/p/q/r/s/t/u` classes). |
| `.../aprsutils/qConstruct` | Apply the APRS-IS `q`-construct algorithm (path rewriting, loop/duplicate detection). |
| `.../aprsutils/client` | Connect to an APRS-IS server over TCP (full stream) or UDP (submit). |
| `.../aprsutils/utils` | Small string helpers used across the library. |

---

## Top-level helpers (`aprsutils`)

### Passcode

```go
code := aprsutils.Passcode("N0CALL") // APRS-IS login passcode for a callsign
```

The passcode is derived from the callsign root (SSID stripped, upper-cased,
truncated to 8 characters).

### Callsign validation

```go
ok := aprsutils.ValidateCallsign("N0CALL-9") // true
```

### Base91

```go
n, err := aprsutils.ToDecimal("<*e7")  // Base91 text -> integer
s, err := aprsutils.FromDecimal(12345) // integer -> Base91 text
s, err = aprsutils.FromDecimal(123, 4) // zero/"!"-padded to a fixed width
```

### Distance

Great-circle / geodesic distance between two `lat,lon` pairs, returned in
**kilometres**:

```go
km := aprsutils.CalculateDistanceVincentyInverse(lat1, lon1, lat2, lon2) // WGS-84, high accuracy
km = aprsutils.CalculateDistanceHaversine(lat1, lon1, lat2, lon2)        // spherical approximation
```

`CalculateDistanceVincentyInverse` returns `NaN` if the iteration fails to
converge (near-antipodal points).

### Logger

The library logs through a small interface so callers can plug in their own
logger (e.g. `zap`):

```go
type Logger interface {
	Debug(context.Context, ...any)
	Info(context.Context, ...any)
	Warn(context.Context, ...any)
	Error(context.Context, ...any)
}
```

`aprsutils.NewLogger()` returns a default implementation that writes to stdout.

---

## parser

Parse a raw APRS-IS line into a structured value.

```go
import "github.com/APRSCN/aprsutils/parser"

p, err := parser.Parse("N0CALL>APRS,TCPIP*:!4903.50N/07201.75W-Test")
if err != nil {
	// malformed packet
}

fmt.Println(p.From)        // "N0CALL"
fmt.Println(p.To)          // "APRS"
fmt.Println(p.Path)        // ["TCPIP*"]
fmt.Println(p.Lat, p.Lon)  // 49.0583, -72.0291
fmt.Println(p.HasPosition) // true
```

`Parsed` exposes the source/destination callsigns, digipeater path, position,
symbol, comment, object/item names, weather, telemetry, message fields and a
`PacketType` bitmask used by type filters.

### PacketType

`PacketType` is a bitmask describing the packet category; test it with `Has`:

```go
if p.PacketType.Has(parser.TypePosition) { /* ... */ }
```

Available bits include `TypePosition`, `TypeObject`, `TypeItem`, `TypeMessage`,
`TypeQuery`, `TypeStatus`, `TypeTelemetry`, `TypeUserDef`, `TypeWeather`,
`TypeNWS`, `TypeBulletin`, `TypeThirdParty`, `TypeNMEA` and `TypeCWOP`.

### Options

```go
// Skip validation of the destination (tocall) field; useful for lenient
// server-side parsing of arbitrary inbound traffic.
p, err := parser.Parse(raw, parser.WithDisableToCallsignValidate())
```

---

## filter

Compile an APRS-IS server filter once, then evaluate it against parsed packets.

```go
import "github.com/APRSCN/aprsutils/filter"

f := filter.Compile("r/33/-96/100 t/m") // within 100 km of 33,-96 OR messages
if f.Match(&p, nil) {
	// packet passes the filter
}
```

`Compile` returns `*Filter`, which is safe to reuse across packets. `Match`
applies negated terms first, then positive terms (matching the reference
APRS-IS ordering).

### Stateful filters (m/, f/, t/ ranges)

Filters that depend on station positions (e.g. `m/` "my range", `f/` "friend
range") need a position source. Pass a `Context`:

```go
type Context interface {
	ClientPosition() (filter.Position, bool)          // the connecting client's last position
	StationPosition(call string) (filter.Position, bool) // any station's last position
}
```

Pass `nil` when no positional state is available; such filters then simply do
not match.

Supported filter classes: `a` (area), `b` (budlist, exact), `d` (digipeater),
`e` (entry station), `f` (friend range), `g` (group/message-to), `m` (my
range), `o`/`os` (object/strict object), `p` (prefix), `q` (q-construct),
`r` (range), `s` (symbol, three layers), `t` (type, incl. `t/c` CWOP),
`u` (unproto destination). Each term may be negated with a leading `-`.

---

## qConstruct

Apply the APRS-IS `q`-construct algorithm: rewrite the `q`-path, detect loops
and duplicate logins, and decide whether a packet should be dropped.

```go
import "github.com/APRSCN/aprsutils/qConstruct"

cfg := &qConstruct.QConfig{
	ServerLogin:    "MYSRV",
	ClientLogin:    "N0CALL",
	ConnectionType: qConstruct.ConnectionVerified,
	IsVerified:     true,
}

res, err := qConstruct.QConstruct(p, cfg)
if err != nil || res.ShouldDrop || res.IsLoop {
	// drop the packet
	return
}

// Splice the new path back into the raw line.
raw, err = qConstruct.Replace(raw, p.To, res.Path)
```

`ConnectionType` selects the handling rules; the available types are
`ConnectionDirectUDP`, `ConnectionUnverified`, `ConnectionVerifiedClientOnly`,
`ConnectionVerified`, `ConnectionOutboundServer`, `ConnectionSendOnly` and
`ConnectionClientOnly`.

`QResult` reports the rewritten `Path`, whether the packet `ShouldDrop` (with a
`DropReason`) and whether it is a routing loop (`IsLoop`).

`Replace` rewrites only the header (path) segment of the raw line, leaving the
payload untouched.

---

## client

An APRS-IS client supporting two transports:

- **TCP** — a persistent stream: login handshake, packet receive loop,
  heartbeat and (optional) automatic reconnect.
- **UDP** — "submit" mode: each datagram is prefixed with the login line so the
  server can authenticate and inject packets independently. There is no receive
  loop in UDP mode.

```go
import "github.com/APRSCN/aprsutils/client"

c := client.NewClient(
	"N0CALL", "12345",          // callsign, passcode
	client.IGate, client.TCP,   // mode, protocol
	"rotate.aprs.net", 14580,   // host, port
	client.WithFilter("r/33/-96/100"),
	client.WithHandler(func(packet string) {
		fmt.Println("RX:", packet)
	}),
)

if err := c.Connect(); err != nil {
	log.Fatal(err)
}
defer c.Close()

_ = c.SendPacket("N0CALL>APRS:>hello from aprsutils")

c.Wait() // block until the client is closed
```

### Modes and protocols

```go
client.Fullfeed // receive everything (no filter)
client.IGate    // receive filtered traffic (use WithFilter)

client.TCP      // persistent stream
client.UDP      // submit-only datagrams
```

### Options

| Option | Effect |
|---|---|
| `WithLogger(l)` | Use a custom `aprsutils.Logger`. |
| `WithHandler(fn)` | Callback for each received packet (TCP). |
| `WithSoftwareAndVersion(name, ver)` | Advertise software name/version in the login line. |
| `WithFilter(spec)` | Server-side filter to request (igate mode). |
| `WithRetryTimes(n)` | Reconnect attempts after a drop (`0` disables internal retry). |
| `WithBufSize(n)` | Read buffer size in bytes. |

### Accessors

`Callsign`, `Filter`, `Mode`, `Protocol`, `Host`, `Port`, `Up`, `Uptime`,
`Server` (upstream software banner), `ServerID` (upstream callsign from the
`logresp` line), `RemoteAddr` (resolved IP:port of the current session) and
`GetStats` (byte/packet counters and rates).

---

## Testing

```sh
go test ./...
go test -race ./...
go vet ./...
```

## License

See [LICENSE](LICENSE).
