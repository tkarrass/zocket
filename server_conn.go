package zocket

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"time"
)

// ServerConnection is the serverside part of the ws abstraction
type ServerConnection struct {
	conn net.Conn // The underlying REAL connection.
	buffered []byte
}

// Read gets some bytes from a ws frame. Blocks.
func (c ServerConnection) Read(b []byte) (int, error) {
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
	buf := &bytes.Buffer{}//
	datloop:
	for {
		tFrame, err := ReadFrame(c.conn)
		if err != nil {
		return 0, err
	}
		switch tFrame.Opcode {
		case FrameType_Ping:
			// send pong
			tFrame.Opcode = FrameType_Pong
			tFrame.WriteTo(c.conn)
		case FrameType_Pong:
			// decide

		case FrameType_Close:
			c.Close()
			return 0, errors.New(fmt.Sprintf("connection closed: %v", tFrame.Payload[:]))
		case FrameType_Binary:
			buf.Write(tFrame.Payload)
			if tFrame.Fin {
				break datloop
			}
		default:
			logrus.Errorf("uhandled frame type: %v", tFrame.Opcode)
		}
	}
	bytes := buf.Bytes()
	for i, _ := range b {
		if i >= len(bytes) {
			return i, nil
		}
		b[i] = bytes[i]
	}
	if len(bytes) > len(b) {
		c.buffered = bytes[len(b):]
	}
	return len(b), nil
}

// Write puts some bytes packed into websocket frames on the wire.
func (c ServerConnection) Write(b []byte) (int, error) {
	tFrame := &Frame{
		Fin:     true,
		Opcode:  FrameType_Binary,
		Masked:  false,
		Mask:    [4]byte{},
		Len:     uint64(len(b)),
		Payload: b,
	}
	err := tFrame.WriteTo(c.conn)
	return len(b), err
}

// Close terminates the connection nicely.
func (c ServerConnection) Close() error {
	logrus.Debugf("serverconn_close %v", c)
	//debug.PrintStack()
	tFrame := &Frame{
		Fin:     true,
		Opcode:  FrameType_Close,
		Masked:  false,
		Mask:    [4]byte{},
		Len:     0,
		Payload: nil,
	}
	_ = tFrame.WriteTo(c.conn)
	return c.conn.Close()
}

func (c ServerConnection) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c ServerConnection) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c ServerConnection) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c ServerConnection) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c ServerConnection) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
