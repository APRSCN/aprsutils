package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APRSCN/aprsutils"
)

// Mode is a ENUM type for client mode
type Mode string

const (
	Fullfeed Mode = "fullfeed"
	IGate    Mode = "igate"
)

// Protocol is a ENUM type for client protocol
type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// Stats contains statistics for the client
type Stats struct {
	TotalSentBytes  uint64
	TotalRecvBytes  uint64
	CurrentSentRate uint64
	CurrentRecvRate uint64
	PacketsSent     uint64
	PacketsReceived uint64
	ConnectionTime  time.Duration
	LastActivity    time.Time
}

// Client provides a basic struct of Client object
type Client struct {
	callsign   string
	passcode   string
	filter     string
	mode       Mode
	protocol   Protocol
	host       string
	port       int
	uptime     time.Time
	up         bool
	retryTimes int
	logger     aprsutils.Logger
	handler    func(packet string)
	server     string // server software banner
	serverID   string // server callsign from logresp
	software   string
	version    string

	conn    net.Conn
	bufSize int

	// readTimeout is the per-read deadline applied while receiving from the
	// server (0 means the built-in default of 30s).
	readTimeout time.Duration

	// TCP keepalive parameters for the connection. When kaEnable is true they
	// are applied to the connected TCP socket so a dead idle peer is detected.
	kaEnable   bool
	kaIdle     time.Duration
	kaInterval time.Duration
	kaCount    int

	// localAddrV4 / localAddrV6 optionally bind the local source address used
	// for outbound connections, selected by the remote address family. Empty
	// means let the OS choose.
	localAddrV4 string
	localAddrV6 string

	// udpLogin is the precomputed "user ... \r\n" line prefixed to every UDP
	// submit datagram (empty for TCP).
	udpLogin string

	mu     sync.Mutex
	done   chan struct{}
	closed bool
	// doneOnce guards close(done) so it happens exactly once, whether it is
	// triggered by Close() or by receivePackets giving up on reconnection.
	doneOnce sync.Once

	// bgStarted ensures the lifecycle-scoped background goroutines (stats
	// updater and heartbeat) are launched exactly once, so reconnects do not
	// leak a fresh copy of each on every attempt.
	bgStarted sync.Once

	// Statistics. Cumulative totals and the current-second accumulators are
	// atomic so the receive/send paths update them directly, without locking
	// or spawning a goroutine per packet. The derived per-second rates are
	// computed once a second by updateStats and published atomically.
	totalSentBytes  atomic.Uint64
	totalRecvBytes  atomic.Uint64
	packetsSent     atomic.Uint64
	packetsReceived atomic.Uint64
	currentSent     atomic.Uint64 // bytes sent in the current 1s window
	currentRecv     atomic.Uint64 // bytes received in the current 1s window
	currentSentRate atomic.Uint64 // last computed send rate (bytes/s)
	currentRecvRate atomic.Uint64 // last computed recv rate (bytes/s)
	lastActivity    atomic.Int64  // unix nanoseconds of last send/recv (0 = none)

	// statsMu guards lastStatsUpdate, which is normally touched only by the
	// single updateStats goroutine but may also be reset by ResetStats.
	statsMu         sync.Mutex
	lastStatsUpdate time.Time
}

// Export data

func (c *Client) Callsign() string {
	return c.callsign
}

func (c *Client) Filter() string {
	return c.filter
}

func (c *Client) Mode() Mode {
	return c.mode
}

func (c *Client) Protocol() Protocol {
	return c.protocol
}

func (c *Client) Host() string {
	return c.host
}

func (c *Client) Port() int {
	return c.port
}

func (c *Client) Uptime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.uptime
}

func (c *Client) Up() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.up
}

func (c *Client) Server() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.server
}

// ServerID returns the upstream server's callsign, parsed from its logresp
// line (empty until login completes).
func (c *Client) ServerID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.serverID
}

// RemoteAddr returns the resolved remote address of the active connection
// (e.g. "44.135.0.1:10152"), or "" if not connected. Unlike Host(), which is
// the configured (possibly DNS) hostname, this reflects the actual IP a
// rotating DNS name resolved to for the current session.
func (c *Client) RemoteAddr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return ""
	}
	return c.conn.RemoteAddr().String()
}

// GetStats returns the current statistics
func (c *Client) GetStats() Stats {
	if c == nil {
		return Stats{}
	}

	s := Stats{
		TotalSentBytes:  c.totalSentBytes.Load(),
		TotalRecvBytes:  c.totalRecvBytes.Load(),
		CurrentSentRate: c.currentSentRate.Load(),
		CurrentRecvRate: c.currentRecvRate.Load(),
		PacketsSent:     c.packetsSent.Load(),
		PacketsReceived: c.packetsReceived.Load(),
	}

	// Connection time (only meaningful while up).
	c.mu.Lock()
	up, uptime := c.up, c.uptime
	c.mu.Unlock()
	if up && !uptime.IsZero() {
		s.ConnectionTime = time.Since(uptime)
	}

	if na := c.lastActivity.Load(); na != 0 {
		s.LastActivity = time.Unix(0, na)
	}

	return s
}

// ResetStats resets all statistics to zero
func (c *Client) ResetStats() {
	c.totalSentBytes.Store(0)
	c.totalRecvBytes.Store(0)
	c.packetsSent.Store(0)
	c.packetsReceived.Store(0)
	c.currentSent.Store(0)
	c.currentRecv.Store(0)
	c.currentSentRate.Store(0)
	c.currentRecvRate.Store(0)

	c.statsMu.Lock()
	c.lastStatsUpdate = time.Now()
	c.statsMu.Unlock()
}

// Option provides a basic option type
type Option func(*Client)

// WithLogger sets default logger to custom
func WithLogger(logger aprsutils.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithHandler sets default packet handler to custom
func WithHandler(handler func(packet string)) Option {
	return func(c *Client) {
		c.handler = handler
	}
}

// WithSoftwareAndVersion sets default software name and version to custom
func WithSoftwareAndVersion(software string, version string) Option {
	return func(c *Client) {
		c.software = software
		c.version = version
	}
}

// WithFilter sets a filter to the client
func WithFilter(filter string) Option {
	return func(c *Client) {
		c.filter = filter
	}
}

// WithRetryTimes sets how many times the client tries to reconnect itself
// after the link drops. Set it to 0 to disable internal reconnection entirely:
// in that mode the client does not reconnect on its own, and when the link
// drops it releases Wait() so an external supervisor can dial a fresh
// connection (see Wait for the full contract).
func WithRetryTimes(retryTimes int) Option {
	return func(c *Client) {
		c.retryTimes = retryTimes
	}
}

// WithBufSize sets a custom buf size for reader
func WithBufSize(bufSize int) Option {
	return func(c *Client) {
		c.bufSize = bufSize
	}
}

// WithReadTimeout sets the per-read deadline applied while receiving from the
// server. A zero or negative value keeps the built-in default.
func WithReadTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.readTimeout = d
		}
	}
}

// WithKeepAlive enables TCP keepalive on the connection: probing starts after
// the socket has been idle for idle, probes are sent every interval, and the
// link is dropped after count failed probes. It has no effect on UDP.
func WithKeepAlive(idle, interval time.Duration, count int) Option {
	return func(c *Client) {
		c.kaEnable = true
		c.kaIdle = idle
		c.kaInterval = interval
		c.kaCount = count
	}
}

// WithLocalAddr binds the local source address for outbound connections. v4 is
// used when connecting to an IPv4 remote, v6 for IPv6; either may be empty to
// let the OS choose for that family.
func WithLocalAddr(v4, v6 string) Option {
	return func(c *Client) {
		c.localAddrV4 = v4
		c.localAddrV6 = v6
	}
}

// NewClient creates a new APRS client
func NewClient(
	callsign string, passcode string,
	mode Mode, protocol Protocol,
	host string, port int,
	options ...Option,
) *Client {
	// Create client
	c := &Client{
		callsign:        callsign,
		passcode:        passcode,
		mode:            mode,
		protocol:        protocol,
		host:            host,
		port:            port,
		software:        aprsutils.Name,
		version:         aprsutils.Version,
		done:            make(chan struct{}),
		lastStatsUpdate: time.Now(),
	}

	// Check callsign
	if callsign == "" {
		c.callsign = "N0CALL"
	}

	// Load default logger
	c.logger = aprsutils.NewLogger()

	// Set default handler
	c.handler = c.handlePacket

	// Set default retry times
	c.retryTimes = 5

	// Set default buf size
	c.bufSize = 1024

	// Apply options
	for _, option := range options {
		option(c)
	}

	return c
}

// Connect to an APRS server.
//
// For TCP the client opens a persistent stream, performs the login handshake
// and starts the receive/heartbeat goroutines. For UDP it opens a connected
// datagram socket used in "UDP submit" mode: there is no login handshake or
// receive loop; instead every SendPacket datagram is prefixed with the login
// line (see SendPacket).
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check client closed
	if c.closed {
		return errors.New("client is closed")
	}

	// Build address
	address := net.JoinHostPort(c.host, strconv.Itoa(c.port))

	network := "tcp"
	if c.protocol == UDP {
		network = "udp"
	}

	conn, err := c.dial(network, address)
	if err != nil {
		return err
	}
	c.up = true
	c.uptime = time.Now()
	c.lastActivity.Store(time.Now().UnixNano())

	c.conn = conn
	c.logger.Info(context.TODO(), "Connected to ", address, " (", string(c.protocol), ")")

	if c.protocol == UDP {
		// Start the lifecycle stats updater once (UDP has no heartbeat).
		c.bgStarted.Do(func() { go c.updateStats() })
		// UDP submit is connectionless and one-way; no handshake/receive loop.
		// The login line is sent with each datagram in SendPacket.
		c.precomputeUDPLogin()
		return nil
	}

	// Enable TCP keepalive if requested so an idle but dead upstream is
	// detected by the OS rather than only via the read timeout.
	if c.kaEnable {
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetKeepAliveConfig(net.KeepAliveConfig{
				Enable:   true,
				Idle:     c.kaIdle,
				Interval: c.kaInterval,
				Count:    c.kaCount,
			})
		}
	}

	// Start the lifecycle-scoped background goroutines exactly once. They live
	// for the whole Client lifetime (until Close closes c.done) and survive
	// reconnects, so starting them per-Connect would leak a goroutine on every
	// reconnect attempt.
	c.bgStarted.Do(func() {
		go c.updateStats()
		go c.heartBeat()
	})

	// Login and start the (connection-scoped) receive loop.
	return c.login()
}

// dial opens a connection to address, optionally binding a configured local
// source address chosen by the resolved remote address family.
func (c *Client) dial(network, address string) (net.Conn, error) {
	dialer := net.Dialer{}

	if c.localAddrV4 != "" || c.localAddrV6 != "" {
		if la := c.localAddrFor(network, address); la != nil {
			dialer.LocalAddr = la
		}
	}

	return dialer.Dial(network, address)
}

// localAddrFor resolves the remote address to determine its family and returns
// the matching configured local address (or nil if none configured for that
// family, or resolution fails — in which case the OS picks the source).
func (c *Client) localAddrFor(network, address string) net.Addr {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil
	}

	// Pick the configured local address matching the first resolved family.
	local := ""
	if ips[0].To4() != nil {
		local = c.localAddrV4
	} else {
		local = c.localAddrV6
	}
	if local == "" {
		return nil
	}

	ip := net.ParseIP(local)
	if ip == nil {
		return nil
	}

	if strings.HasPrefix(network, "udp") {
		return &net.UDPAddr{IP: ip}
	}
	return &net.TCPAddr{IP: ip}
}

// precomputeUDPLogin builds the login line prepended to each UDP datagram.
func (c *Client) precomputeUDPLogin() {
	passcodeString := ""
	if c.passcode != "" {
		passcodeString = fmt.Sprintf(" pass %s", c.passcode)
	}
	login := fmt.Sprintf("user %s%s vers %s %s", c.callsign, passcodeString, c.software, c.version)
	if c.mode != Fullfeed && c.filter != "" {
		login += fmt.Sprintf(" filter %s", c.filter)
	}
	c.udpLogin = login + "\r\n"
}

// Login to an APRS server
func (c *Client) login() error {
	// Construct login string
	passcodeString := ""
	if c.passcode != "" {
		passcodeString = fmt.Sprintf(" pass %s", c.passcode)
	}
	loginStr := fmt.Sprintf("user %s%s vers %s %s", c.callsign, passcodeString, c.software, c.version)
	// Maybe have a filter?
	if c.mode != Fullfeed && c.filter != "" {
		loginStr += fmt.Sprintf(" filter %s", c.filter)
	}
	loginStr += "\r\n"

	// Send login request
	sent, err := c.conn.Write([]byte(loginStr))
	if err != nil {
		c.logger.Error(context.TODO(), "Error writing login command to ", c.conn.RemoteAddr().String(), err)
		return err
	}

	// Update statistics
	c.addSentBytes(sent)

	// Check passcode
	if strconv.Itoa(aprsutils.Passcode(c.callsign)) == c.passcode {
		c.logger.Info(context.TODO(), "Logged in as ", c.callsign)
	}

	// Start packet receiving for this connection. The stats updater and
	// heartbeat are lifecycle-scoped and started once by Connect.
	go c.receivePackets()

	return nil
}

// addSentBytes records bytes written to the server (direct atomic update).
func (c *Client) addSentBytes(bytes int) {
	if bytes <= 0 {
		return
	}
	c.totalSentBytes.Add(uint64(bytes))
	c.currentSent.Add(uint64(bytes))
	c.lastActivity.Store(time.Now().UnixNano())
}

// addRecvBytes records bytes read from the server (direct atomic update).
func (c *Client) addRecvBytes(bytes int) {
	if bytes <= 0 {
		return
	}
	c.totalRecvBytes.Add(uint64(bytes))
	c.currentRecv.Add(uint64(bytes))
	c.lastActivity.Store(time.Now().UnixNano())
}

// updateStats periodically updates the current rate statistics
func (c *Client) updateStats() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			now := time.Now()

			c.statsMu.Lock()
			elapsed := now.Sub(c.lastStatsUpdate).Seconds()
			c.lastStatsUpdate = now
			c.statsMu.Unlock()

			if elapsed > 0 {
				// Swap out the current-window accumulators and convert to a
				// per-second rate.
				sent := c.currentSent.Swap(0)
				recv := c.currentRecv.Swap(0)
				c.currentSentRate.Store(uint64(float64(sent) / elapsed))
				c.currentRecvRate.Store(uint64(float64(recv) / elapsed))
			}
		}
	}
}

// internalHandler handles packet first to do statistic
func (c *Client) internalHandler(packet string) {
	c.packetsReceived.Add(1)
	c.handler(packet)
}

// receivePackets receives packet from the APRS server. When the link drops it
// attempts up to retryTimes reconnections; if it cannot re-establish the link
// (or retryTimes is 0, i.e. reconnection is owned by an external supervisor)
// it signals the client done so a blocked Wait() returns. A successful
// reconnect hands the lifecycle to the fresh receive loop, so this one returns
// without signalling done.
func (c *Client) receivePackets() {
	// reconnected is set when a successful Connect() has handed the lifecycle
	// to a new receive loop; in that case we must not signal done. On every
	// other return path the link is permanently down, so we release Wait().
	reconnected := false
	defer func() {
		if !reconnected {
			c.signalDone()
		}
	}()

	// Create a reader
	reader := bufio.NewReaderSize(c.conn, c.bufSize)

	readTimeout := c.readTimeout
	if readTimeout <= 0 {
		readTimeout = 30 * time.Second
	}

	serverInfoCount := 0
root:
	for {
		select {
		case <-c.done:
			return
		default:
			// Set timeout
			if err := c.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
				c.logger.Error(context.TODO(), "Error setting read deadline (timeout) ", err)
				break root
			}

			// Read string from reader
			line, err := reader.ReadString('\n')
			if err != nil {
				if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
					// Timeout, retry
					continue
				}
				if err.Error() == "EOF" {
					c.logger.Warn(context.TODO(), "Server closed the connection")
					break root
				}
				c.logger.Error(context.TODO(), "Error reading from server ", err)
				break root
			}

			// Update received bytes statistics
			c.addRecvBytes(len(line))

			// Trim space
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Check prefix
			if strings.HasPrefix(line, "#") {
				c.logger.Debug(context.TODO(), "Server info: ", line)
				// server/serverID are read by the accessors from other
				// goroutines, so publish them under the lock.
				c.mu.Lock()
				if serverInfoCount == 0 {
					c.server = strings.TrimPrefix(line, "# ")
				}
				// Parse the server callsign from "... server <ID>".
				if i := strings.LastIndex(line, "server "); i >= 0 {
					id := strings.TrimSpace(line[i+len("server "):])
					if id != "" {
						c.serverID = id
					}
				}
				c.mu.Unlock()
				serverInfoCount++
				continue
			}

			// Handle packet
			c.internalHandler(line)
		}
	}

	// Update status
	c.mu.Lock()
	c.up = false
	c.mu.Unlock()

	// Check closed
	select {
	case <-c.done:
		return
	default:
	}

	// Debounce
	time.Sleep(1 * time.Second)

	// Reconnect
	for i := 0; i < c.retryTimes; i++ {
		// Check closed
		select {
		case <-c.done:
			return
		default:
		}

		if err := c.Connect(); err != nil {
			c.logger.Error(context.TODO(), "Error connecting to server", err, " retry ", i)
			time.Sleep(3 * time.Second)
			continue
		} else {
			// A fresh receive loop now owns the client lifecycle; do not
			// signal done here.
			reconnected = true
			return
		}
	}
}

// handlePacket handles APRS packet that has received
func (c *Client) handlePacket(packet string) {
	parts := strings.SplitN(packet, ">", 2)
	if len(parts) < 2 {
		return
	}

	sender := parts[0]
	remaining := parts[1]

	pathData := strings.SplitN(remaining, ":", 2)
	if len(pathData) < 2 {
		return
	}

	path := pathData[0]
	data := pathData[1]

	c.logger.Debug(context.TODO(), "Raw packet received: ", packet)
	c.logger.Info(context.TODO(), "APRS packet - Sender: ", sender, ", Path: ", path, ", Data: ", data)
}

// SendPacket sends an APRS packet.
//
// For TCP the packet is written to the stream terminated by CRLF. For UDP
// (submit mode) the datagram is the login line followed by the packet, both
// CRLF-terminated, so the server can authenticate and inject each datagram
// independently (qAU).
func (c *Client) SendPacket(packet string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.closed {
		return errors.New("client is closed or not connected")
	}

	// Construct datagram/stream payload.
	var fullPacket string
	if c.protocol == UDP {
		fullPacket = c.udpLogin + packet + "\r\n"
	} else {
		fullPacket = packet + "\r\n"
	}

	sent, err := c.conn.Write([]byte(fullPacket))
	if err != nil {
		c.logger.Error(context.TODO(), "Error send packet: ", err)
		return err
	}

	// Update statistics
	c.addSentBytes(sent)
	c.packetsSent.Add(1)

	c.logger.Debug(context.TODO(), "Sent packet: ", packet)
	return nil
}

// heartBeat sends a keepalive periodically for the whole client lifetime. It
// is started once (see Connect) and survives reconnects: while the link is
// down (conn == nil) it simply skips a tick; it only exits when the client is
// closed.
func (c *Client) heartBeat() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			// Skip while disconnected; exit only when the client is closed.
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				return
			}
			if c.conn == nil {
				c.mu.Unlock()
				continue
			}
			c.mu.Unlock()

			ping := fmt.Sprintf("# %s keepalive %d", c.software, time.Now().Unix())
			if err := c.SendPacket(ping); err != nil {
				c.logger.Error(context.TODO(), "Heartbeat failed, connection may be closed")

				// Drop the dead connection so the receive loop reconnects; do
				// not exit — the heartbeat resumes once the link is back.
				c.mu.Lock()
				if c.conn != nil {
					_ = c.conn.Close()
					c.conn = nil
					c.up = false
				}
				c.mu.Unlock()
			}
		}
	}
}

// Close a client
func (c *Client) Close() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	c.signalDone()

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.logger.Error(context.TODO(), "Error closing connection ", err)
		} else {
			c.logger.Info(context.TODO(), "client closed")
		}
		c.conn = nil
	}
}

// Wait blocks until the client is permanently done, i.e. the link has dropped
// and the client will not (re)connect on its own. It returns when either:
//
//   - Close() is called, or
//   - the connection dropped and the client exhausted its reconnection
//     attempts (including the WithRetryTimes(0) case, where the client does no
//     internal reconnection at all — see signalDone in receivePackets).
//
// This is the contract relied on by an external supervisor (e.g. the aprsgo
// uplink manager) that owns reconnection: it sets WithRetryTimes(0) and uses
// Wait() to learn when the link is down so it can dial a fresh connection.
func (c *Client) Wait() {
	<-c.done
}

// signalDone closes c.done exactly once. It marks the client as permanently
// finished so a blocked Wait() returns. Unlike Close it does not tear down the
// (already dead) connection; it is the path taken when receivePackets stops
// reconnecting.
func (c *Client) signalDone() {
	c.doneOnce.Do(func() { close(c.done) })
}
