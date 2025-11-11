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

	conn net.Conn

	mu     sync.Mutex
	done   chan struct{}
	closed bool
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

// NewClient creates a new APRS client
func NewClient(
	callsign string, passcode string,
	mode Mode, protocol Protocol,
	host string, port int,
	options ...Option,
) *Client {
	// Create client
	c := &Client{
		callsign: callsign,
		passcode: passcode,
		mode:     mode,
		protocol: protocol,
		host:     host,
		port:     port,
		software: aprsutils.Name,
		version:  aprsutils.Version,
		done:     make(chan struct{}),
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

	c.conn = conn
	c.logger.Info(nil, "Connected to ", address)

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
	_, err := c.conn.Write([]byte(loginStr))
	if err != nil {
		c.logger.Error(nil, "Error writing login command to ", c.conn.RemoteAddr().String(), err)
		return err
	}

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

// receivePackets receives packet from the APRS server
func (c *Client) receivePackets() {
	// Create a reader
	reader := bufio.NewReader(c.conn)

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
			c.handler(line)
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
	_, err := c.conn.Write([]byte(fullPacket))
	if err != nil {
		c.logger.Error(nil, "Error send packet: ", err)
		return err
	}

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
