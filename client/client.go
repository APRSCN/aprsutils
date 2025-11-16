package client

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
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
	TotalSentBytes  uint64        `json:"totalSentBytes"`
	TotalRecvBytes  uint64        `json:"totalRecvBytes"`
	CurrentSentRate uint64        `json:"currentSentRate"`
	CurrentRecvRate uint64        `json:"currentRecvRate"`
	PacketsSent     uint64        `json:"packetsSent"`
	PacketsReceived uint64        `json:"packetsReceived"`
	ConnectionTime  time.Duration `json:"connectionTime"`
	LastActivity    time.Time     `json:"lastActivity"`
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
	server     string
	software   string
	version    string

	conn    net.Conn
	bufSize int

	mu     sync.Mutex
	done   chan struct{}
	closed bool

	// Statistics fields
	statsMu         sync.RWMutex
	stats           Stats
	currentSent     uint64
	currentRecv     uint64
	lastStatsUpdate time.Time
	lastActivity    time.Time
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
	return c.uptime
}

func (c *Client) Up() bool {
	return c.up
}

func (c *Client) Server() string {
	return c.server
}

// GetStats returns the current statistics
func (c *Client) GetStats() Stats {
	if c == nil {
		return Stats{}
	}

	c.statsMu.RLock()
	defer c.statsMu.RUnlock()

	// Update connection time
	if c.up && !c.uptime.IsZero() {
		c.stats.ConnectionTime = time.Since(c.uptime)
	}

	// Update last activity
	if !c.lastActivity.IsZero() {
		c.stats.LastActivity = c.lastActivity
	}

	return c.stats
}

// ResetStats resets all statistics to zero
func (c *Client) ResetStats() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	c.stats = Stats{}
	c.currentSent = 0
	c.currentRecv = 0
	c.lastStatsUpdate = time.Now()
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

// WithRetryTimes sets a retry times to custom
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

// Connect to an APRS server
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check client closed
	if c.closed {
		return errors.New("client is closed")
	}

	// Build address
	address := net.JoinHostPort(c.host, strconv.Itoa(c.port))

	// Try to create TCP connection
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	c.up = true
	c.uptime = time.Now()
	c.lastActivity = time.Now()

	c.conn = conn
	c.logger.Info(nil, "Connected to ", address)

	// Start statistics updater
	go c.updateStats()

	// Return and login
	return c.login()
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
		c.logger.Error(nil, "Error writing login command to ", c.conn.RemoteAddr().String(), err)
		return err
	}

	// Update statistics
	go c.updateSentBytesStats(sent)

	// Check passcode
	if strconv.Itoa(aprsutils.Passcode(c.callsign)) == c.passcode {
		c.logger.Info(nil, "Logged in as ", c.callsign)
	}

	// Start packet receiving
	go c.receivePackets()

	// Start heartbeat
	go c.heartBeat()

	return nil
}

// updateSentBytesStats updates sent bytes statistics
func (c *Client) updateSentBytesStats(bytes int) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats.TotalSentBytes += uint64(bytes)
	c.currentSent += uint64(bytes)
	c.lastActivity = time.Now()
}

// updateSentPacketStats updates sent packets statistics
func (c *Client) updateSentPacketStats(packet int) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats.PacketsSent += uint64(packet)
}

// updateRecvBytesStats updates received bytes statistics
func (c *Client) updateRecvBytesStats(bytes int) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats.TotalRecvBytes += uint64(bytes)
	c.currentRecv += uint64(bytes)
	c.stats.PacketsReceived += 1
	c.lastActivity = time.Now()
}

// updateRecvPacketStats updates received packets statistics
func (c *Client) updateRecvPacketStats(packet int) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.stats.PacketsReceived += uint64(packet)
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
			c.statsMu.Lock()
			now := time.Now()
			elapsed := now.Sub(c.lastStatsUpdate).Seconds()

			if elapsed > 0 {
				// Calculate current rates
				currentSent := c.currentSent
				c.currentSent = 0
				currentRecv := c.currentRecv
				c.currentRecv = 0

				c.stats.CurrentSentRate = uint64(float64(currentSent) / elapsed)
				c.stats.CurrentRecvRate = uint64(float64(currentRecv) / elapsed)
			}

			c.lastStatsUpdate = now
			c.statsMu.Unlock()
		}
	}
}

// internalHandler handles packet first to do statistic
func (c *Client) internalHandler(packet string) {
	go c.updateRecvPacketStats(1)
	c.handler(packet)
}

// receivePackets receives packet from the APRS server
func (c *Client) receivePackets() {
	// Create a reader
	reader := bufio.NewReaderSize(c.conn, c.bufSize)

	serverInfoCount := 0
root:
	for {
		select {
		case <-c.done:
			return
		default:
			// Set timeout
			err := c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			if err != nil {
				c.logger.Error(nil, "Error setting read deadline (timeout) ", err)
				break root
			}

			// Read string from reader
			line, err := reader.ReadString('\n')
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					// Timeout, retry
					continue
				}
				if err.Error() == "EOF" {
					c.logger.Warn(nil, "Server closed the connection")
					break root
				}
				c.logger.Error(nil, "Error reading from server ", err)
				break root
			}

			// Update received bytes statistics
			go c.updateRecvBytesStats(len(line))

			// Trim space
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Check prefix
			if strings.HasPrefix(line, "#") {
				c.logger.Debug(nil, "Server info: ", line)
				if serverInfoCount == 0 {
					c.server = strings.TrimPrefix(line, "# ")
				}
				serverInfoCount++
				continue
			}

			// Handle packet
			c.internalHandler(line)
		}
	}

	// Update status
	c.up = false

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

		err := c.Connect()
		if err != nil {
			c.logger.Error(nil, "Error connecting to server", err, " retry ", i)
			time.Sleep(3 * time.Second)
			continue
		} else {
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

	c.logger.Debug(nil, "Raw packet received: ", packet)
	c.logger.Info(nil, "APRS packet - Sender: ", sender, ", Path: ", path, ", Data: ", data)
}

// SendPacket sends an APRS packet
func (c *Client) SendPacket(packet string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.closed {
		return errors.New("client is closed or not connected")
	}

	// Construct packet
	fullPacket := packet + "\r\n"
	sent, err := c.conn.Write([]byte(fullPacket))
	if err != nil {
		c.logger.Error(nil, "Error send packet: ", err)
		return err
	}

	// Update statistics
	go c.updateSentBytesStats(sent)
	go c.updateSentPacketStats(1)

	c.logger.Debug(nil, "Sent packet: ", packet)
	return nil
}

// heartBeat sends heart beat to keep alive
func (c *Client) heartBeat() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			// Check connection
			c.mu.Lock()
			if c.conn == nil || c.closed {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()

			ping := fmt.Sprintf("# %s keepalive %d", c.software, time.Now().Unix())
			err := c.SendPacket(ping)
			if err != nil {
				c.logger.Error(nil, "Heartbeat failed, connection may be closed")

				// Close connection
				c.mu.Lock()
				if c.conn != nil {
					_ = c.conn.Close()
					c.conn = nil
					c.up = false
				}
				c.mu.Unlock()
				return
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
	close(c.done)

	if c.conn != nil {
		err := c.conn.Close()
		if err != nil {
			c.logger.Error(nil, "Error closing connection ", err)
		} else {
			c.logger.Info(nil, "client closed")
		}
		c.conn = nil
	}
}

// Wait the client exit
func (c *Client) Wait() {
	<-c.done
}
