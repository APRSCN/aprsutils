package client

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/APRSCN/aprsutils"
)

// Types is a ENUM type for client type
type Types string

const (
	Fullfeed Types = "fullfeed"
	IGate    Types = "igate"
)

// Protocol is a ENUM type for client protocol
type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// Client provides a basic struct of client object
type Client struct {
	Callsign   string `json:"callsign"`
	passcode   string
	Filter     string   `json:"filter"`
	Type       Types    `json:"type"`
	Protocol   Protocol `json:"protocol"`
	Host       string   `json:"host"`
	Port       int      `json:"port"`
	retryTimes int
	logger     aprsutils.Logger
	handler    func(packet string)
	software   string
	version    string
	conn       net.Conn
	done       chan bool
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
		c.Filter = filter
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
	typ Types, protocol Protocol,
	host string, port int,
	options ...Option,
) *Client {
	// Create client
	client := &Client{
		Callsign: callsign,
		passcode: passcode,
		Type:     typ,
		Protocol: protocol,
		Host:     host,
		Port:     port,
		software: aprsutils.Name,
		version:  aprsutils.Version,
	}

	// Check callsign
	if callsign == "" {
		client.Callsign = "N0CALL"
	}

	// Load default logger
	client.logger = aprsutils.NewLogger()

	// Set default handler
	client.handler = client.handlePacket

	// Set default retry times
	client.retryTimes = 5

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// Connect to an APRS server
func (c *Client) Connect() error {
	// Build address
	address := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))

	// Try to create TCP connection
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}

	c.conn = conn
	c.logger.Info(nil, "Connected to", address)

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
	loginStr := fmt.Sprintf("user %s%s vers %s %s", c.Callsign, passcodeString, c.software, c.version)
	// Maybe have a filter?
	if c.Type != Fullfeed && c.Filter != "" {
		loginStr += fmt.Sprintf(" filter %s", c.Filter)
	}
	loginStr += "\r\n"

	// Send login request
	_, err := c.conn.Write([]byte(loginStr))
	if err != nil {
		c.logger.Error(nil, "Error writing login command to ", c.conn.RemoteAddr().String(), err)
		return err
	}

	// Check passcode
	if strconv.Itoa(aprsutils.Passcode(c.Callsign)) == c.passcode {
		c.logger.Info(nil, "Logged in as", c.Callsign)
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

	bk := false
	for {
		if bk {
			break
		}
		select {
		case <-c.done:
			return
		default:
			// Set timeout
			err := c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			if err != nil {
				c.logger.Error(nil, "Error setting read deadline (timeout)", err)
				bk = true
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
					bk = true
				}
				c.logger.Error(nil, "Error reading from server", err)
				bk = true
			}

			// Trim space
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Check prefix
			if strings.HasPrefix(line, "#") {
				c.logger.Info(nil, "Server info:", line)
				continue
			}

			// Handle packet
			c.handler(line)
		}
	}

	// Reconnect
	for i := 0; i < c.retryTimes; i++ {
		err := c.Connect()
		if err != nil {
			c.logger.Error(nil, "Error connecting to server", err, "retry", i)
			continue
		}
		time.Sleep(5 * time.Second)
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

	c.logger.Debug(nil, "Raw packet received:", packet)
	c.logger.Info(nil, "APRS packet - Sender:", sender, ", Path:", path, ", Data:", data)
}

// SendPacket sends an APRS packet
func (c *Client) SendPacket(packet string) error {
	// Construct packet
	fullPacket := packet + "\r\n"
	_, err := c.conn.Write([]byte(fullPacket))
	if err != nil {
		c.logger.Error(nil, "Error send packet:", err)
		return err
	}

	c.logger.Debug(nil, "Sent packet:", packet)
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
			ping := fmt.Sprintf("# %s keepalive %d", c.software, time.Now().Unix())
			_ = c.SendPacket(ping)
		}
	}
}

// Close a client
func (c *Client) Close() {
	close(c.done)
	for {
		if c.conn != nil {
			err := c.conn.Close()
			if err != nil {
				c.logger.Error(nil, "Error closing connection", err)
				continue
			}
			c.logger.Info(nil, "Client closed")
			break
		}
	}
}
