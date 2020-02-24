package zocket

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
)

// A Listener as defined in net.Listener for accepting WebSocket connections.
type Listener struct {
	cConn chan net.Conn
}

// NewListener creates a new listener for WebSocket connections.
// You need to register the Listener as a HTTP handler in order for it to become operative.
func NewListener() Listener {
	return Listener{
		cConn: make(chan net.Conn),
	}
}

// Implement the net.Listener interface

// Accept waits for and returns the next connection to the listener.
//
// The Connection may be treated like any other streaming network connection,
// but wraps the traffic internally to WebSocket frames.
//
// This function blocks.
func (l Listener) Accept() (net.Conn, error) {
	tConn, ok := <-l.cConn
	if !ok {
		return nil, errors.New("socket closed")
	}
	ret := ServerConnection{
		conn: tConn,
	}
	return ret, nil
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l Listener) Close() error {
	select {
		case _, ok := <-l.cConn:
			// either the channel closed or we just read a connection from it.
			// we discard the connection, since we're about to close the channel anyways
			if !ok {
				return errors.New("channel already closed")
			}
		default:
			// something blocks. the channel is alive
	}
	close(l.cConn)
	return nil
}

// Addr returns the listener's network address.
// By their nature, WebSocket connections are upgraded HTTP connections and don't have
// a real end point address. So a default address is always returned.
func (l Listener) Addr() net.Addr {
	return addr
}

// Implement http handler interface

// ServeHTTP handles a WebSocket upgrade request.
// You should make sure, a call to this function is really done on an update request,
// since it would respond with a http error otherwise.
func (l Listener) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// Sanity checks:
	//  - method is HTTP GET
	//  - has request URI
	//  - has Host header -> no!
	//  - has Connection: Upgrade
	//  - has Upgrade: websocket
	//  - has Sec-WebSocket-Version: 13
	//  - has Sec-WebSocket-Key
	//  - Origin: is optional
	//  - Sec-WebSocket-Protocol is optional
	//  - Sec-WebSocket-Extensions is optional

	if req.Method != "GET" {
		log.Error("Invalid method!")
		resp.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if req.RequestURI == "" {
		log.Errorf("Invalid Request URI: '%v'", req.RequestURI)
		resp.WriteHeader(http.StatusNotAcceptable) // ???
		return
	}

	//if req.Header.Get("Host") == "" {
	//	log.Error("No Host")
	//	resp.WriteHeader(http.StatusExpectationFailed) // ???
	//	return
	//}

	if req.Header.Get("Connection") != "Upgrade" {
		log.Error("Connection != Upgrade")
		resp.WriteHeader(http.StatusUpgradeRequired)
		return
	}

	if req.Header.Get("Upgrade") != "websocket" {
		log.Error("Upgrade != websocket")
		resp.WriteHeader(http.StatusUpgradeRequired)
		return
	}

	if req.Header.Get("Sec-WebSocket-Version") != "13" {
		log.Error("Invalid WebSocket Version")
		resp.WriteHeader(http.StatusUpgradeRequired)
		return
	}

	// Todo: remove all that debug spam
	log.Warn("In the websocket handler - yay!")
	for k, v := range req.Header {
		log.Infof("   %v: %v", k, v[0])
	}

	wsKey := req.Header.Get("Sec-Websocket-Key")
	wsKey += "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	shakey := sha1.Sum([]byte(wsKey))
	wsKey = base64.StdEncoding.EncodeToString(shakey[:])

	log.Info("Generated WebSocket-Accept: ", wsKey)
	// NOPE: we need to hijack the connection and send it manually!
	//resp.Header().Set("Upgrade", "websocket")
	//resp.Header().Set("Connection", "Upgrade")
	//resp.Header().Set("Sec-WebSocket-Accept", wsKey)
	//resp.WriteHeader(101)

	h, ok := resp.(http.Hijacker)
	if !ok {
		log.Error("Missing Hijacker extension on responsewriter")
		return
	}
	var brw *bufio.ReadWriter    // only used to check for data sent by the client (which is disallowed)
	conn, brw, err := h.Hijack() // returns the connection and the readwriter to operate on
	if err != nil {
		log.WithError(err).Error("cannot hijack")
		return
	}

	buf := &bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%v 101 Switching Protocols\r\n", req.Proto)) // per RFC: HTTP/1.1 or higher
	buf.WriteString("Upgrade: websocket\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString(fmt.Sprintf("Sec-WebSocket-Accept: %v\r\n\r\n", wsKey))

	// the client must not've sent any data before the handshake is complete
	if brw.Reader.Buffered() > 0 {
		conn.Close()
		log.Error("client send before handshake!")
		return
	}

	// fire!
	conn.Write(buf.Bytes())

	l.cConn <- conn
}
