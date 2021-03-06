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

package wireguard

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ivpn/desktop-app-daemon/service/dns"
	"github.com/ivpn/desktop-app-daemon/shell"
	"github.com/ivpn/desktop-app-daemon/vpn"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

var (
	// we are using same service name for WireGuard connection
	// Therefore, we must ensure that only one connection (service) is currently active
	_globalInitMutex sync.Mutex
)

type operation int

const (
	pause  operation = iota
	resume operation = iota
)

// internalVariables of wireguard implementation for macOS
type internalVariables struct {
	manualDNS             net.IP
	isRestartRequired     bool           // if true - connection will be restarted
	pauseRequireChan      chan operation // control connection pause\resume or disconnect from paused state
	isDisconnectRequested bool
	isPaused              bool
}

const (
	// such significant delays required to support ultimate slow PC
	_waitServiceInstallTimeout = time.Minute * 3
	_waitServiceStartTimeout   = time.Minute * 5
)

func (wg *WireGuard) init() error {
	// uninstall WG service (if installed)

	if installed, err := wg.isServiceInstalled(); installed == false || err != nil {
		if err != nil {
			return err
		}
		return nil // service not available (so, nothing to uninstall)
	}

	log.Warning("The IVPN WireGuard service is installed (it is not expected). Uninstalling it...")
	return wg.uninstallService()
}

// connect - SYNCHRONOUSLY execute openvpn process (wait until it finished)
func (wg *WireGuard) connect(stateChan chan<- vpn.StateInfo) error {
	if wg.internals.isDisconnectRequested {
		return fmt.Errorf("disconnection already requested for this object. To make a new connection, please, initialize new one")
	}

	defer func() {
		wg.internals.pauseRequireChan = nil
		// do not forget to remove manual DNS configuration (if necessary)
		if err := dns.DeleteManual(nil); err != nil {
			log.Error(err)
		}
		log.Info("Connection stopped")
	}()

	err := wg.disconnectInternal()
	if err != nil {
		return fmt.Errorf("failed to disconnect before new connection: %w", err)
	}

	// connect to service maneger
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to windows service manager : %w", err)
	}
	defer m.Disconnect()

	// install WireGuard service
	err = wg.installService(stateChan)
	if err != nil {
		return fmt.Errorf("failed to install windows service: %w", err)
	}

	// CONNECTED

	if wg.internals.isDisconnectRequested {
		// there is chance that disconnection request come during WG was establishing connection
		// in this case - perform disconnection
		log.Info("Disconnection was requested")
		return wg.uninstallService()
	}

	wg.internals.pauseRequireChan = make(chan operation, 1)

	// this method is synchronous. Waiting until service stop
	// (periodically checking of service status)
	// TODO: Probably we should avoid checking the service state in a loop (with constant delay). Think about it.
	for ; ; time.Sleep(time.Millisecond * 50) {
		_, stat, err := wg.getServiceStatus(m)
		if err != nil {
			if err == windows.ERROR_SERVICE_DOES_NOT_EXIST || err == windows.ERROR_SERVICE_DISABLED || err == windows.ERROR_SERVICE_NOT_ACTIVE || err == windows.ERROR_SERVICE_NOT_FOUND {
				break
			}
		}

		if stat == svc.Stopped {
			break
		}

		// PAUSE\RESUME
		select {
		case toDoOperation := <-wg.internals.pauseRequireChan:
			if toDoOperation == pause {
				wg.internals.isPaused = true
				defer func() {
					// do not forget to mark connection as resumed
					wg.internals.isPaused = false
				}()

				log.Info("Pausing...")

				if err := wg.uninstallService(); err != nil {
					log.Error("failed to pause connection (disconnetion error):", err.Error())
					return err
				}

				log.Info("Paused")

				// waiting to resume or stop request
				for {
					toDoOperation = <-wg.internals.pauseRequireChan
					if toDoOperation != pause { // ignore consequent 'pause' requests
						break
					}
				}

				if wg.internals.isDisconnectRequested {
					break
				}

				if toDoOperation == resume {
					log.Info("Resuming...")

					if err := wg.installService(stateChan); err != nil {
						log.Error("failed to resume connection (new connetion error):", err.Error())
						return err
					}

					// reconnected successfully
					wg.internals.isPaused = false
					log.Info("Resumed")
					break
				}
			}
		default:
			// no pause required
		}

		// Check is reconnection required
		// It can happen when configuration parameters were changed (e.g. ManualDNS value)
		if wg.internals.isRestartRequired {
			wg.internals.isRestartRequired = false

			stateChan <- vpn.NewStateInfo(vpn.RECONNECTING, "Reconnecting with new connection parameters")

			log.Info("Restarting...")
			if err := wg.uninstallService(); err != nil {
				log.Error("failed to restart connection (disconnetion error):", err.Error())
			} else {
				if err := wg.installService(stateChan); err != nil {
					log.Error("failed to restart connection (new connetion error):", err.Error())
				} else {
					// reconnected successfully
					log.Info("Connection restarted")
				}
			}
		}
	}

	return nil
}

func (wg *WireGuard) disconnect() error {
	wg.internals.isDisconnectRequested = true
	return wg.disconnectInternal()
}

func (wg *WireGuard) disconnectInternal() error {
	log.Info("Disconnecting...")

	wg.requireOperation(resume) // resume (if we are in paused state)

	return wg.uninstallService()
}

func (wg *WireGuard) isPaused() bool {
	return wg.internals.isPaused
}

func (wg *WireGuard) pause() error {
	wg.requireOperation(pause)
	return nil
}

func (wg *WireGuard) resume() error {
	wg.requireOperation(resume)
	return nil
}

func (wg *WireGuard) requireOperation(o operation) error {
	ch := wg.internals.pauseRequireChan
	if ch != nil {
		ch <- o
	}
	return nil
}

func (wg *WireGuard) setManualDNS(addr net.IP) error {
	if addr.Equal(wg.internals.manualDNS) {
		return nil
	}

	wg.internals.manualDNS = addr

	if running, err := wg.isServiceRunning(); err != nil || running == false {
		return err
	}

	log.Info("Connection will be restarted due to DNS server IP configuration change...")
	// request a restart with new connection parameters
	wg.internals.isRestartRequired = true

	return nil
}

func (wg *WireGuard) resetManualDNS() error {
	if wg.internals.manualDNS == nil {
		return nil
	}

	wg.internals.manualDNS = nil

	if running, err := wg.isServiceRunning(); err != nil || running == false {
		return err
	}

	log.Info("Connection will be restarted due to DNS server IP configuration change...")
	// request a restart with new connection parameters
	wg.internals.isRestartRequired = true

	return nil
}

func (wg *WireGuard) getTunnelName() string {
	return strings.TrimSuffix(filepath.Base(wg.configFilePath), filepath.Ext(wg.configFilePath)) // IVPN
}

func (wg *WireGuard) getServiceName() string {
	return "WireGuardTunnel$" + wg.getTunnelName() // WireGuardTunnel$IVPN
}

func (wg *WireGuard) getOSSpecificConfigParams() (interfaceCfg []string, peerCfg []string) {

	manualDNS := wg.internals.manualDNS
	if manualDNS != nil {
		interfaceCfg = append(interfaceCfg, "DNS = "+manualDNS.String())
	} else {
		interfaceCfg = append(interfaceCfg, "DNS = "+wg.connectParams.hostLocalIP.String())
	}

	interfaceCfg = append(interfaceCfg, "Address = "+wg.connectParams.clientLocalIP.String())

	// TODO: check if we need it for this platform
	// Same as "0.0.0.0/0" but such type of configuration is disabling internal WireGuard-s Firewall
	// It blocks everything except WireGuard traffic.
	// We need to disable WireGuard-s firewall because we have our own implementation of firewall.
	//  For details, refer to WireGuard-windows sources: tunnel\ifaceconfig.go (enableFirewall(...) method)
	peerCfg = append(peerCfg, "AllowedIPs = 128.0.0.0/1, 0.0.0.0/1")

	return interfaceCfg, peerCfg
}

func (wg *WireGuard) getServiceStatus(m *mgr.Mgr) (bool, svc.State, error) {
	service, err := m.OpenService(wg.getServiceName())
	if err != nil {
		return false, 0, err
	}
	defer service.Close()

	// read service state
	stat, err := service.Control(svc.Interrogate)
	if err != nil {
		return true, 0, err
	}
	return true, stat.State, nil
}

func (wg *WireGuard) isServiceRunning() (bool, error) {
	// connect to service maneger
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer m.Disconnect()

	// looking for service
	serviceName := wg.getServiceName()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false, nil // service not available
	}
	s.Close()

	_, stat, err := wg.getServiceStatus(m)
	if err != nil {
		return false, err
	}

	if stat == svc.Running {
		return true, nil
	}

	return false, nil
}

// install WireGuard service
func (wg *WireGuard) installService(stateChan chan<- vpn.StateInfo) error {
	isInstalled := false
	isStarted := false

	defer func() {
		if isStarted == false || isInstalled == false {
			log.Info("Failed to install service. Uninstalling...")
			err := wg.disconnectInternal()
			if err != nil {
				log.Error("Failed to uninstall service after unsuccessful connect: ", err.Error())
			}
		}
	}()

	// NO parallel operations of serviceInstall OR serviceUninstall should be performed!
	_globalInitMutex.Lock()
	defer func() {
		_globalInitMutex.Unlock()
	}()

	log.Info("Connecting...")

	// generate configuration
	defer os.Remove(wg.configFilePath)
	err := wg.generateAndSaveConfigFile(wg.configFilePath)
	if err != nil {
		return fmt.Errorf("failed to save config file: %w", err)
	}

	// start service
	log.Info("Installing service...")
	err = shell.Exec(nil, wg.binaryPath, "/installtunnelservice", wg.configFilePath)
	if err != nil {
		return fmt.Errorf("failed to install WireGuard service: %w", err)
	}

	// connect to service maneger
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect windows service manager: %w", err)
	}
	defer m.Disconnect()

	// waiting for until service installed
	log.Info("Waiting for service install...")
	serviceName := wg.getServiceName()
	for started := time.Now(); time.Since(started) < _waitServiceInstallTimeout; time.Sleep(time.Millisecond * 10) {
		service, err := m.OpenService(serviceName)
		if err == nil {
			log.Info("Service installed")
			service.Close()
			isInstalled = true
			break
		}
	}

	// service install timeout
	if isInstalled == false {
		return fmt.Errorf("service not installed (timeout)")
	}

	// wait for service starting
	log.Info("Waiting for service start...")
	for started := time.Now(); time.Since(started) < _waitServiceStartTimeout; time.Sleep(time.Millisecond * 10) {
		_, stat, err := wg.getServiceStatus(m)
		if err != nil {
			return fmt.Errorf("service start error: %w", err)
		}

		if stat == svc.Running {
			log.Info("Service started")
			isStarted = true
			break
		} else if stat == svc.Stopped {
			return fmt.Errorf("WireGuard service stopped")
		}
	}

	if isStarted == false {
		return fmt.Errorf("service not started (timeout)")
	}

	// WireGuard interface is configured to correct DNS.
	// But we must to be sure if non-ivpn interfaces are configured to our DNS
	// (it needed ONLY if DNS IP located in local network)
	// Also, it is needed to inform 'dns' package about last DNS value (used by 'protocol' to ptovide dns status to clients)
	manualDNS := wg.internals.manualDNS
	if manualDNS != nil {
		dns.SetManual(manualDNS, nil)
	} else {
		// delete manual DNS (if defined)
		dns.DeleteManual(nil)
	}

	// CONNECTED
	log.Info("Connection started")
	// Send 'connected' notification only after 'dns' package informed about correct DNS value
	wg.notifyConnectedStat(stateChan)

	return nil
}

// uninstall WireGuard service
func (wg *WireGuard) isServiceInstalled() (bool, error) {
	// connect to service maneger
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("failed to connect windows service manager: %w", err)
	}
	defer m.Disconnect()

	// looking for service
	serviceName := wg.getServiceName()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false, nil // service not available
	}
	s.Close()

	return true, nil
}

// uninstall WireGuard service
func (wg *WireGuard) uninstallService() error {
	// NO parallel operations of serviceInstall OR serviceUninstall should be performed!
	_globalInitMutex.Lock()
	defer _globalInitMutex.Unlock()

	// connect to service maneger
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect windows service manager: %w", err)
	}
	defer m.Disconnect()

	// looking for service
	serviceName := wg.getServiceName()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return nil // service not available (so, nothing to uninstall)
	}
	s.Close()

	log.Info("Uninstalling service...")
	// stop service
	err = shell.Exec(nil, wg.binaryPath, "/uninstalltunnelservice", wg.getTunnelName())
	if err != nil {
		return fmt.Errorf("failed to uninstall WireGuard service: %w", err)
	}

	lastUninstallRetryTime := time.Now()
	nextUninstallRetryTime := time.Second * 3

	isUninstalled := false
	for started := time.Now(); time.Since(started) < _waitServiceInstallTimeout && isUninstalled == false; time.Sleep(50) {
		isServFound, state, err := wg.getServiceStatus(m)
		if err != nil {
			if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
				isUninstalled = true
				break
			}
		}

		// Sometimes a call "/uninstalltunnelservice" has no result
		// Here we are retrying to perform uninstall request (retry interval is increasing each time)
		if isServFound && state == svc.Running && time.Since(lastUninstallRetryTime) > nextUninstallRetryTime {
			log.Info("Retry: uninstalling service...")
			err = shell.Exec(nil, wg.binaryPath, "/uninstalltunnelservice", wg.getTunnelName())
			if err != nil {
				return fmt.Errorf("failed to uninstall WireGuard service: %w", err)
			}
			lastUninstallRetryTime = time.Now()
			nextUninstallRetryTime = nextUninstallRetryTime * 2
		}
	}

	if isUninstalled == false {
		return fmt.Errorf("service not uninstalled (timeout)")
	}

	log.Info("Service uninstalled")
	return nil
}

func (wg *WireGuard) onRoutingChanged() error {
	// do nothing for Windows
	return nil
}
