//
//  Daemon for IVPN Client Desktop
//  https://github.com/ivpn/desktop-app-daemon
//
//  Created by Stelnykovych Alexandr.
//  Copyright (c) 2020 Privatus Limited.
//
//  This file is part of the Daemon for IVPN Client Desktop.
//
//  The Daemon for IVPN Client Desktop is free software: you can redistribute it and/or
//  modify it under the terms of the GNU General Public License as published by the Free
//  Software Foundation, either version 3 of the License, or (at your option) any later version.
//
//  The Daemon for IVPN Client Desktop is distributed in the hope that it will be useful,
//  but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
//  or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for more
//  details.
//
//  You should have received a copy of the GNU General Public License
//  along with the Daemon for IVPN Client Desktop. If not, see <https://www.gnu.org/licenses/>.
//

package vpn

import (
	"errors"
	"net"
	"strings"
)

// Type - VPN type
type Type int

// Supported VPN protocols
const (
	OpenVPN   Type = iota
	WireGuard Type = iota
)

func (t Type) String() string {
	switch t {
	case OpenVPN:
		return "OpenVPN"
	case WireGuard:
		return "WireGuard"
	}
	return "<Unknown>"
}

// State - state of VPN
type State int

// Possible VPN state values (must be applicable for all protocols)
// Such stetes MUST be in use by ALL supportded VPN protocols:
// 		DISCONNECTED
// 		CONNECTING
// 		CONNECTED
// 		EXITING
const (
	DISCONNECTED State = iota
	CONNECTING   State = iota // OpenVPN's initial state.
	WAIT         State = iota // (Client only) Waiting for initial response from server.
	AUTH         State = iota // (Client only) Authenticating with server.
	GETCONFIG    State = iota // (Client only) Downloading configuration options from server.
	ASSIGNIP     State = iota // Assigning IP address to virtual network interface.
	ADDROUTES    State = iota // Adding routes to system.
	CONNECTED    State = iota // Initialization Sequence Completed.
	RECONNECTING State = iota // A restart has occurred.
	TCP_CONNECT  State = iota // TCP_CONNECT
	EXITING      State = iota // A graceful exit is in progress.
)

func (s State) String() string {
	if s < DISCONNECTED || s > EXITING {
		return "<Unknown>"
	}

	return []string{
		"DISCONNECTED",
		"CONNECTING",
		"WAIT",
		"AUTH",
		"GETCONFIG",
		"ASSIGNIP",
		"ADDROUTES",
		"CONNECTED",
		"RECONNECTING",
		"TCP_CONNECT",
		"EXITING"}[s]
}

// ParseState - Converts string representation of OpenVPN state to vpn.State
func ParseState(stateStr string) (State, error) {
	stateStr = strings.Trim(stateStr, " \t;,.")
	switch stateStr {
	case "CONNECTING":
		return CONNECTING, nil
	case "WAIT":
		return WAIT, nil
	case "AUTH":
		return AUTH, nil
	case "GET_CONFIG":
		return GETCONFIG, nil
	case "ASSIGN_IP":
		return ASSIGNIP, nil
	case "ADD_ROUTES":
		return ADDROUTES, nil
	case "CONNECTED":
		return CONNECTED, nil
	case "RECONNECTING":
		return RECONNECTING, nil
	case "TCP_CONNECT":
		return TCP_CONNECT, nil
	case "EXITING":
		return EXITING, nil
	default:
		return DISCONNECTED, errors.New("unexpected state:" + stateStr)
	}
}

// StateInfo - VPN state + additional information
type StateInfo struct {
	State       State
	Description string

	VpnType      Type
	Time         int64  // unix time (seconds)
	IsTCP        bool   // applicable only for 'CONNECTED' state
	ClientIP     net.IP // applicable only for 'CONNECTED' state
	ClientPort   int    // applicable only for 'CONNECTED' state (source port)
	ServerIP     net.IP // applicable only for 'CONNECTED' state
	ServerPort   int    // applicable only for 'CONNECTED' state (destination port)
	ExitServerID string // applicable only for 'CONNECTED' state
	IsCanPause   bool   // applicable only for 'CONNECTED' state
	IsAuthError  bool   // applicable only for 'EXITING' state

	// TODO: try to avoid using this protocol-specific parameter in future
	// Currently, in use by OpenVPN connection to inform about "RECONNECTING" reason (e.g. "tls-error", "init_instance"...)
	// UI client using this info in order to determine is it necessary to try to connect with another port
	StateAdditionalInfo string
}

// NewStateInfo - create new state object (not applicable for CONNECTED state)
func NewStateInfo(state State, description string) StateInfo {
	return StateInfo{
		State:       state,
		Description: description,
		ClientIP:    nil,
		ServerIP:    nil,
		IsAuthError: false}
}

// NewStateInfoConnected - create new state object for CONNECTED state
func NewStateInfoConnected(isTCP bool, clientIP net.IP, localPort int, serverIP net.IP, destPort int, isCanPause bool) StateInfo {
	return StateInfo{
		State:       CONNECTED,
		Description: "",
		IsTCP:       isTCP,
		ClientIP:    clientIP,
		ClientPort:  localPort,
		ServerIP:    serverIP,
		ServerPort:  destPort,
		IsAuthError: false,
		IsCanPause:  isCanPause}
}

// Process represents VPN object operations
type Process interface {
	// Type just returns VPN type
	Type() Type
	// Init performs basic initializations before connection
	// It is usefull, for example, for WireGuard(Windows) - to ensure that WG service is fully uninstalled
	// (currently, in use by WireGuard(Windows))
	Init() error

	// Connect - SYNCHRONOUSLY execute openvpn process (wait until it finished)
	Connect(stateChan chan<- StateInfo) error
	Disconnect() error
	Pause() error
	Resume() error
	IsPaused() bool

	SetManualDNS(addr net.IP) error
	ResetManualDNS() error

	// DestinationIP -  Get destination IP (VPN host server or proxy server IP address)
	// This information if required, for example, to allow this address in firewall
	DestinationIP() net.IP
}
