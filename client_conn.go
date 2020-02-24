// +build js, wasm

package zocket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime/debug"
	"sync"
	"syscall/js"
	"time"
)

type handler func(this js.Value, args []js.Value) interface{}

// ClientConnection is a TCP like Connection between the wasm and a server.
type ClientConnection struct {
	M sync.Mutex
	ws js.Value
	conn chan struct{}
	in chan []byte
	onMessage handler
	buffered []byte
}

func makeHandler(c chan []byte) handler {
	return func(this js.Value, args []js.Value) interface{} {
		e := args[0]
		array := e.Get("data") // is an arraybuffer
		u8arr := js.Global().Get("Uint8Array").New(array)
		//fmt.Printf("arr: %v, %+v\n", u8arr.Type(), u8arr)
		bytes := make([]byte, u8arr.Get("byteLength").Int())
		/* ln := */ js.CopyBytesToGo(bytes, u8arr)
		//fmt.Printf("got %v bytes [ID: %v]\n", ln, e.Get("lastEventId").String())
		//fmt.Printf("message: %+v\n", bytes)
		c <- bytes
		return nil
	}
}

func Dial(ctx context.Context, target string) (net.Conn, error) {
	fmt.Printf("dialing %v\n", target)
	ret := ClientConnection{
		ws: js.Global().Get("WebSocket").New(target),
		in: make(chan []byte),
		conn: make(chan struct{}),
	}
	ret.ws.Set("binaryType", "arraybuffer")
	ret.onMessage = makeHandler(ret.in)

	ret.ws.Call("addEventListener", "open", js.FuncOf(
		func(this js.Value, args []js.Value) interface{} {
			fmt.Println("Opened")
			close(ret.conn)
			return nil
		}))

	ret.ws.Call("addEventListener", "error", js.FuncOf(
		func(this js.Value, args []js.Value) interface{} {
			fmt.Println("error")
			return nil
		}))

	ret.ws.Call("addEventListener", "close", js.FuncOf(
		func(this js.Value, args []js.Value) interface{} {
			fmt.Println("close")
			return nil
		}))

	// MessageEvent: [all ro]
	//  .data   - the data sent
	//  .origin
	//  .lastEventId
	//  .source -
	ret.ws.Call("addEventListener", "message", js.FuncOf(ret.onMessage))

	<-ret.conn // block until the connection is really open
	return ret, nil
}


// Read gets some bytes from a ws frame. Blocks.
func (c ClientConnection) Read(b []byte) (int, error) {
	fmt.Println("client_read")
	c.M.Lock()
	defer c.M.Unlock()
	if len(c.buffered) > 0 {
		for i, _ := range b {
			if i >= len(c.buffered) {
				c.buffered = nil
				return i, nil
			}
			b[i] = c.buffered[i]
		}
		if len(c.buffered) > len(b) {
			c.buffered = c.buffered[len(b):]
		}
		return len(b), nil
	}
	var err error = nil
	bytes, ok := <-c.in
	if !ok {
		err = errors.New("read from closed connection")
	}
	for i, _ := range b {
		if i >= len(bytes) {
			fmt.Printf("read %v bytes\n", i)
			return i, nil
		}
		b[i] = bytes[i]
	}
	if len(bytes) > len(b) {
		c.buffered = bytes[len(b):]
	}
	fmt.Printf("read %v bytes\n", len(b))
	return len(b), err
}

// Write puts some bytes packed into websocket frames on the wire.
func (c ClientConnection) Write(b []byte) (int, error) {
	array := js.Global().Get("Uint8Array").New(len(b))
	n := js.CopyBytesToJS(array, b)
	c.ws.Call("send", array)
	return n, nil
}

// Close terminates the connection nicely.
func (c ClientConnection) Close() error {
	fmt.Println("client_close")
	debug.PrintStack()
	c.ws.Call("close")
	return nil
}

// LocalAddr returns the adress of the local endpoint. Since we operate within a sandbox we don't have any. return a
// dummy and make underlying layers think they're using a tcp conection.
func (c ClientConnection) LocalAddr() net.Addr {
	fmt.Println("DBG: call LocalAddr() on ClientConnection")
	return Addr {
		network: "tcp",
		address: "0.0.0.0",
	}
}

// RemoteAddr returns the adress of the remote endpoint.
func (c ClientConnection) RemoteAddr() net.Addr {
	fmt.Println("DBG: call RemoteAddr() on ClientConnection")
	return Addr {
		network: "tcp",
		// TODO: Use the real servers adress somehow (or none at all? :think:)
		address: "127.0.0.1",
	}
}

func (c ClientConnection) SetDeadline(t time.Time) error {
	return errors.New("SetDeadline not implemented")
}

func (c ClientConnection) SetReadDeadline(t time.Time) error {
	return errors.New("SetReadDeadline not implemented")
}

func (c ClientConnection) SetWriteDeadline(t time.Time) error {
	return errors.New("SetWriteDeadline not implemented")
}
