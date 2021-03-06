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

package api

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"time"
)

func (a *API) getAlternateIPs() (lastGoodIP net.IP, ipList []net.IP) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	return a.lastGoodAlternateIP, a.alternateIPs
}

func (a *API) saveLastGoodAlternateIP(lastGoodIP net.IP) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.lastGoodAlternateIP = lastGoodIP
}

func getURL(host string, urlpath string) string {
	return "https://" + path.Join(host, urlpath)
}

func newRequest(urlPath string, method string, contentType string, body io.Reader) (*http.Request, error) {
	if len(method) == 0 {
		method = "GET"
	}

	req, err := http.NewRequest(method, urlPath, body)
	if err != nil {
		return nil, err
	}

	if len(contentType) > 0 {
		req.Header.Add("Content-type", contentType)
	}

	return req, nil
}

func findPinnedKey(certHashes []string, certBase64hash256 string) bool {
	for _, hash := range certHashes {
		if hash == certBase64hash256 {
			return true
		}
	}
	return false
}

type dialer func(network, addr string) (net.Conn, error)

func makeDialer(certHashes []string, skipCAVerification bool, serverName string) dialer {
	return func(network, addr string) (net.Conn, error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error(fmt.Sprintf("PANIC (API request): "), r)
				if err, ok := r.(error); ok {
					log.ErrorTrace(err)
				}
			}
		}()

		tlsConfig := &tls.Config{
			InsecureSkipVerify: skipCAVerification,
			ServerName:         serverName, // only have sense when skipCAVerification == false
		}

		c, err := tls.Dial(network, addr, tlsConfig)
		if err != nil {
			return c, err
		}
		connstate := c.ConnectionState()
		var lastErr error = nil
		for _, peercert := range connstate.PeerCertificates {
			der, err := x509.MarshalPKIXPublicKey(peercert.PublicKey)
			if err != nil {
				lastErr = err
				continue
			}

			hash := sha256.Sum256(der)
			certBase64hash := base64.StdEncoding.EncodeToString(hash[:])

			if err != nil {
				log.Error(err)
			}

			if findPinnedKey(certHashes, certBase64hash) {
				return c, nil // Pinned Key found
			}

		}
		if lastErr != nil {
			return nil, fmt.Errorf("Certificate check error: pinned certificate key not found: %w", lastErr)
		}
		return nil, fmt.Errorf("Certificate check error: pinned certificate key not found")
	}
}

func (a *API) doRequest(host string, urlPath string, method string, contentType string, request interface{}, timeoutMs int) (resp *http.Response, err error) {
	if len(host) == 0 || host == _apiHost {
		return a.doRequestAPIHost(urlPath, method, contentType, request, timeoutMs)
	} else if host == _updateHost {
		return a.doRequestUpdateHost(urlPath, method, contentType, request, timeoutMs)
	}
	return nil, fmt.Errorf("unknown host type")
}

func (a *API) doRequestUpdateHost(urlPath string, method string, contentType string, request interface{}, timeoutMs int) (resp *http.Response, err error) {
	transCfg := &http.Transport{
		// using certificate key pinning
		DialTLS: makeDialer(UpdateIvpnHashes, false, _updateHost),
	}

	// configure http-client with preconfigured TLS transport
	timeout := _defaultRequestTimeout
	if timeoutMs > 0 {
		timeout = time.Millisecond * time.Duration(timeoutMs)
	}
	client := &http.Client{Transport: transCfg, Timeout: timeout}

	data := []byte{}
	if request != nil {
		data, err = json.Marshal(request)
		if err != nil {
			return nil, err
		}
	}

	bodyBuffer := bytes.NewBuffer(data)

	// try to access API server by host DNS
	req, err := newRequest(getURL(_updateHost, urlPath), method, contentType, bodyBuffer)
	if err != nil {
		return nil, err
	}

	resp, e := client.Do(req)
	if e != nil {
		log.Warning("Failed to access " + _updateHost)
		return resp, fmt.Errorf("Unable to access IVPN repo server: %w", e)
	}

	return resp, nil
}

func (a *API) doRequestAPIHost(urlPath string, method string, contentType string, request interface{}, timeoutMs int) (resp *http.Response, err error) {
	lastIP, ips := a.getAlternateIPs()

	// When trying to access API server by alternate IPs (not by DNS name)
	// we need to configure TLS to use api.ivpn.net hostname
	// (to avoid certificate errors)
	transCfg := &http.Transport{
		// NOTE: TLSClientConfig not in use in case of DialTLS defined
		//TLSClientConfig: &tls.Config{
		//	ServerName: _apiHost,
		//},

		// using certificate key pinning
		DialTLS: makeDialer(APIIvpnHashes, false, _apiHost),
	}
	if len(APIIvpnHashes) == 0 {
		log.Warning("No pinned certificates for ", _apiHost)
		transCfg = &http.Transport{
			// NOTE: TLSClientConfig not in use in case of DialTLS defined
			TLSClientConfig: &tls.Config{
				ServerName: _apiHost,
			},
		}
	}

	// configure http-client with preconfigured TLS transport
	timeout := _defaultRequestTimeout
	if timeoutMs > 0 {
		timeout = time.Millisecond * time.Duration(timeoutMs)
	}
	client := &http.Client{Transport: transCfg, Timeout: timeout}

	data := []byte{}
	if request != nil {
		data, err = json.Marshal(request)
		if err != nil {
			return nil, err
		}
	}

	bodyBuffer := bytes.NewBuffer(data)

	// access API by last good IP (if defined)
	if lastIP != nil {
		req, err := newRequest(getURL(lastIP.String(), urlPath), method, contentType, bodyBuffer)
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}
	}

	// try to access API server by host DNS
	req, err := newRequest(getURL(_apiHost, urlPath), method, contentType, bodyBuffer)
	if err != nil {
		return nil, err
	}
	firstResp, firstErr := client.Do(req)
	if firstErr == nil {
		// save last good IP
		a.saveLastGoodAlternateIP(nil)
		return firstResp, firstErr
	}
	log.Warning("Failed to access " + _apiHost)

	// try to access API server by alternate IP
	for i, ip := range ips {
		log.Info(fmt.Sprintf("Trying to use alternate API IP #%d...", i))

		req, err := newRequest(getURL(ip.String(), urlPath), method, contentType, bodyBuffer)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)

		if err != nil {
			fmt.Println("Failed: ", err.Error())
			continue
		}

		// save last good IP
		a.saveLastGoodAlternateIP(ip)

		log.Info(fmt.Sprintf("Success!"))
		return resp, err
	}

	return firstResp, fmt.Errorf("Unable to access IVPN API server: %w", firstErr)
}

func (a *API) requestRaw(host string, urlPath string, method string, contentType string, requestObject interface{}, timeoutMs int) (responseData []byte, err error) {
	resp, err := a.doRequest(host, urlPath, method, contentType, requestObject, timeoutMs)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to get API HTTP response body: %w", err)
	}

	return body, nil
}

func (a *API) request(host string, urlPath string, method string, contentType string, requestObject interface{}, responseObject interface{}) error {
	return a.requestEx(host, urlPath, method, contentType, requestObject, responseObject, 0)
}

func (a *API) requestEx(host string, urlPath string, method string, contentType string, requestObject interface{}, responseObject interface{}, timeoutMs int) error {
	body, err := a.requestRaw(host, urlPath, method, contentType, requestObject, timeoutMs)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, responseObject); err != nil {
		return fmt.Errorf("failed to deserialize API response: %w", err)
	}

	return nil
}
