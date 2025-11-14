package qConstruct

import (
	"fmt"
	"net"
	"strings"

	"github.com/APRSCN/aprsutils"
	"github.com/APRSCN/aprsutils/parser"
)

// ConnectionType defined type of connections
type ConnectionType int

const (
	ConnectionDirectUDP ConnectionType = iota
	ConnectionUnverified
	ConnectionVerifiedClientOnly
	ConnectionVerified
	ConnectionOutboundServer
	ConnectionSendOnly
	ConnectionClientOnly
)

// QConfig includes config of QConstruct
type QConfig struct {
	ServerLogin    string
	ClientLogin    string
	ConnectionType ConnectionType
	EnableTrace    bool
	RemoteIP       string
	IsVerified     bool
	IsClientOnly   bool
	IsSendOnly     bool
}

// QResult is the struct of result of QConstruct
type QResult struct {
	Path       []string
	ShouldDrop bool
	DropReason string
	IsLoop     bool
}

// QConstruct processes QConstruct
func QConstruct(p *parser.Parsed, config *QConfig) (*QResult, error) {
	result := &QResult{
		Path: make([]string, len(p.Path)),
	}
	copy(result.Path, p.Path)

	// Apply initial processing for all packets
	result.applyInitialProcessing(p, config)

	// Check loop before all qConstruct
	if result.checkForLoopsBeforeProcessing(config) {
		return result, nil
	}

	// Process based on connection type
	switch config.ConnectionType {
	case ConnectionDirectUDP:
		result.processDirectUDP(config)
	case ConnectionUnverified:
		result.processUnverified(config, p.From)
	case ConnectionVerifiedClientOnly:
		result.processVerifiedClientOnly(config, p.From)
	case ConnectionVerified, ConnectionSendOnly, ConnectionClientOnly:
		result.processStandardConnection(config, p.From)
	case ConnectionOutboundServer:
		result.processOutboundServer(config)
	}

	// Apply final processing for all packets with q constructs
	result.applyFinalProcessing(config)

	return result, nil
}

// applyInitialProcessing inits to processing packet
func (r *QResult) applyInitialProcessing(p *parser.Parsed, config *QConfig) {
	// Remove q construct if it's last in path with no call following
	if len(r.Path) > 0 {
		lastElement := r.Path[len(r.Path)-1]
		if strings.HasPrefix(lastElement, "q") && len(lastElement) == 3 {
			// q construct with no following call - remove it
			r.Path = r.Path[:len(r.Path)-1]
		}
	}

	// If no q construct and packet from logged-in station
	if !r.hasQConstruct() && strings.EqualFold(p.From, config.ClientLogin) {
		if config.IsVerified {
			r.Path = append(r.Path, "TCPIP*")
		} else {
			r.Path = append(r.Path, "TCPXX*")
		}
	}
}

// checkForLoopsBeforeProcessing checks loop before all qConstruct
func (r *QResult) checkForLoopsBeforeProcessing(config *QConfig) bool {
	// Check for qAZ construct
	if r.hasSpecificQConstruct("qAZ") {
		r.ShouldDrop = true
		r.DropReason = "qAZ construct - server-client command packet"
		return true
	}

	// Check for qAC construct with invalid path
	if r.hasSpecificQConstruct("qAC") && !r.hasTCPIPPath() {
		r.ShouldDrop = true
		r.DropReason = "qAC construct without TCPIP* path"
		return true
	}

	// Check for server login in q construct (loop detection)
	if r.containsServerLogin(config.ServerLogin) {
		r.ShouldDrop = true
		r.IsLoop = true
		r.DropReason = "Loop detected - server login found in q construct"
		return true
	}

	// Check for duplicate callsign-SSID in q construct
	if r.hasDuplicateCallsigns() {
		r.ShouldDrop = true
		r.IsLoop = true
		r.DropReason = "Loop detected - duplicate callsign-SSID in q construct"
		return true
	}

	return false
}

// processDirectUDP processes direct UDP connection
func (r *QResult) processDirectUDP(config *QConfig) {
	qConstructCount := r.countQConstructs()

	if qConstructCount == 1 {
		// Replace existing q construct with qAU
		r.replaceQConstruct("qAU", config.ServerLogin)
	} else if qConstructCount > 1 {
		// Invalid header - drop packet
		r.ShouldDrop = true
		r.DropReason = "Multiple q constructs in UDP packet"
	} else {
		// Append qAU
		r.Path = append(r.Path, "qAU", config.ServerLogin)
	}
}

// processUnverified processes unverified connection
func (r *QResult) processUnverified(config *QConfig, fromCall string) {
	if !strings.EqualFold(fromCall, config.ClientLogin) {
		// Packet not deemed "OK" from unverified connection - drop
		r.ShouldDrop = true
		r.DropReason = "FROMCALL doesn't match login in unverified connection"
		return
	}

	if r.hasQConstruct() {
		r.replaceQConstruct("qAX", config.ServerLogin)
	} else {
		r.Path = append(r.Path, "qAX", config.ServerLogin)
	}
}

// processVerifiedClientOnly processed verified connection
func (r *QResult) processVerifiedClientOnly(config *QConfig, fromCall string) {
	if strings.EqualFold(fromCall, config.ClientLogin) {
		// Should not happen for client-only connections
		return
	}

	if r.hasQConstruct() {
		qConstructIndex := r.findQConstructIndex()
		if qConstructIndex >= 0 && qConstructIndex+1 < len(r.Path) {
			qType := r.Path[qConstructIndex]
			viaCall := r.Path[qConstructIndex+1]

			switch qType {
			case "qAR", "qAr":
				// Replace with qAo
				r.Path[qConstructIndex] = "qAo"
			case "qAS":
				// Replace with qAO
				r.Path[qConstructIndex] = "qAO"
			case "qAC":
				if !strings.EqualFold(viaCall, config.ServerLogin) &&
					!strings.EqualFold(viaCall, config.ClientLogin) {
					r.Path[qConstructIndex] = "qAO"
				}
			}
		}
	} else if len(r.Path) > 1 && strings.HasSuffix(r.Path[len(r.Path)-1], ",I") {
		// Handle ,I construct at the end
		viaCall := strings.TrimSuffix(r.Path[len(r.Path)-1], ",I")
		r.Path = r.Path[:len(r.Path)-1] // Remove the ,I element
		r.Path = append(r.Path, "qAo", viaCall)
	} else {
		// Append qAO with login
		r.Path = append(r.Path, "qAO", config.ClientLogin)
	}
}

// processStandardConnection processed standard connection
func (r *QResult) processStandardConnection(config *QConfig, fromCall string) {
	if r.hasQConstruct() {
		// Skip to "All packets with q constructs"
		return
	}

	// Check for ,I construct at the end
	if len(r.Path) > 0 {
		lastElement := r.Path[len(r.Path)-1]
		if strings.HasSuffix(lastElement, ",I") {
			viaCall := strings.TrimSuffix(lastElement, ",I")
			if strings.EqualFold(viaCall, config.ClientLogin) {
				// Change to qAR
				r.Path[len(r.Path)-1] = "qAR"
				r.Path = append(r.Path, viaCall)
			} else {
				// Change to qAr
				r.Path[len(r.Path)-1] = "qAr"
				r.Path = append(r.Path, viaCall)
			}
			return
		}
	}

	if strings.EqualFold(fromCall, config.ClientLogin) {
		if config.ConnectionType == ConnectionSendOnly {
			r.Path = append(r.Path, "qAO", config.ServerLogin)
		} else {
			r.Path = append(r.Path, "qAC", config.ServerLogin)
		}
	} else {
		r.Path = append(r.Path, "qAS", config.ClientLogin)
	}
}

// processOutboundServer processed outbound connection
func (r *QResult) processOutboundServer(config *QConfig) {
	if r.hasQConstruct() {
		return
	}

	if len(r.Path) > 0 {
		lastElement := r.Path[len(r.Path)-1]
		if strings.HasSuffix(lastElement, ",I") {
			viaCall := strings.TrimSuffix(lastElement, ",I")
			r.Path[len(r.Path)-1] = "qAr"
			r.Path = append(r.Path, viaCall)
		} else {
			// Append qAS with IP address (deprecated)
			ipHex := r.ipToHex(config.RemoteIP)
			r.Path = append(r.Path, "qAS", ipHex)
		}
	}
}

// applyFinalProcessing final processed qConstruct
func (r *QResult) applyFinalProcessing(config *QConfig) {
	if r.ShouldDrop {
		return
	}

	// Handle trace
	if config.EnableTrace || r.hasSpecificQConstruct("qAI") {
		r.applyTrace(config)
	}
}

// applyTrace applied trace
func (r *QResult) applyTrace(config *QConfig) {
	// For verified ports where login is not found after q construct
	if config.ConnectionType == ConnectionVerified && !r.containsLoginAfterQ(config.ClientLogin) {
		r.Path = append(r.Path, config.ClientLogin)
	} else if config.ConnectionType == ConnectionOutboundServer {
		ipHex := r.ipToHex(config.RemoteIP)
		r.Path = append(r.Path, ipHex)
	}

	// Append server login
	r.Path = append(r.Path, config.ServerLogin)
}

// hasQConstruct checks whether the path contained QConstruct
func (r *QResult) hasQConstruct() bool {
	for _, element := range r.Path {
		if strings.HasPrefix(element, "q") && len(element) == 3 {
			return true
		}
	}
	return false
}

// hasSpecificQConstruct checks whether the path contained specific QConstruct
func (r *QResult) hasSpecificQConstruct(qType string) bool {
	for _, element := range r.Path {
		if element == qType {
			return true
		}
	}
	return false
}

// countQConstructs calculates the number of QConstruct included by path
func (r *QResult) countQConstructs() int {
	count := 0
	for _, element := range r.Path {
		if strings.HasPrefix(element, "q") && len(element) == 3 {
			count++
		}
	}
	return count
}

// findQConstructIndex finds index of first QConstruct
func (r *QResult) findQConstructIndex() int {
	for i, element := range r.Path {
		if strings.HasPrefix(element, "q") && len(element) == 3 {
			return i
		}
	}
	return -1
}

// replaceQConstruct replaces QConstruct
func (r *QResult) replaceQConstruct(newQType string, viaCall string) {
	index := r.findQConstructIndex()
	if index >= 0 {
		r.Path[index] = newQType
		if index+1 < len(r.Path) {
			r.Path[index+1] = viaCall
		} else {
			r.Path = append(r.Path, viaCall)
		}
	}
}

// hasTCPIPPath checks whether the path has TCPIP mark
func (r *QResult) hasTCPIPPath() bool {
	for _, element := range r.Path {
		if element == "TCPIP*" {
			return true
		}
	}
	return false
}

// containsServerLogin checks whether the path has server login mark
func (r *QResult) containsServerLogin(serverLogin string) bool {
	for _, element := range r.Path {
		if strings.EqualFold(element, serverLogin) {
			return true
		}
	}
	return false
}

// hasDuplicateCallsigns checks whether there's a duplicate callsign
func (r *QResult) hasDuplicateCallsigns() bool {
	seen := make(map[string]bool)
	for _, element := range r.Path {
		// Simple check for callsign pattern (can be enhanced)
		if aprsutils.ValidateCallsign(element) {
			normalized := strings.ToUpper(element)
			if seen[normalized] {
				return true
			}
			seen[normalized] = true
		}
	}
	return false
}

// containsLoginAfterQ checks whether it has a login symbol after QConstruct
func (r *QResult) containsLoginAfterQ(login string) bool {
	for i, element := range r.Path {
		if strings.HasPrefix(element, "q") && len(element) == 3 {
			// Check if this is the last via call
			if i+1 < len(r.Path) && strings.EqualFold(r.Path[i+1], login) {
				return true
			}
		}
	}
	return false
}

// ipToHex transfer IP address to HEX (8 chars)
func (r *QResult) ipToHex(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "00000000"
	}

	// Use the last 4 bytes for IPv4, or first 8 bytes for IPv6
	if ip.To4() != nil {
		ip = ip.To4()
		return fmt.Sprintf("%02X%02X%02X%02X", ip[0], ip[1], ip[2], ip[3])
	} else {
		// For IPv6, use first 8 bytes
		return fmt.Sprintf("%02X%02X%02X%02X%02X%02X%02X%02X",
			ip[0], ip[1], ip[2], ip[3], ip[4], ip[5], ip[6], ip[7])
	}
}

// GetPathString returns string of path result
func (r *QResult) GetPathString() string {
	return strings.Join(r.Path, ",")
}
