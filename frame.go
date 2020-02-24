package zocket

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
)

type FrameType uint8

const (
	FrameType_Cont   FrameType = 0x00
	FrameType_Text   FrameType = 0x01
	FrameType_Binary FrameType = 0x02
	FrameType_Close  FrameType = 0x08
	FrameType_Ping   FrameType = 0x09
	FrameType_Pong   FrameType = 0x0a
)

/*
 *
 */
type Frame struct {
	Fin     bool
	Opcode  FrameType
	Masked  bool
	Mask    [4]byte
	Len     uint64
	Payload []byte
}

// Read a Frame from the wire.
// Blocks until a complete frame has been retrieved.
func ReadFrame(conn net.Conn) (*Frame, error) {
	tFrame := &Frame{}
	head := make([]byte, 2, 2)
	n, err := conn.Read(head)
	if err != nil {
		return nil, err
	}
	if n != len(head) {
		return nil, errors.New("incomplete header")
	}

	// header sanity checks
	tFrame.Fin = (head[0] & 128) != 0
	rsv1 := (head[0] & 64) != 0
	rsv2 := (head[0] & 32) != 0
	rsv3 := (head[0] & 16) != 0
	if rsv1 || rsv2 || rsv3 {
		return nil, errors.New("invalid frame header")
	}
	tFrame.Opcode = FrameType(head[0] & 15)

	tFrame.Masked = (head[1] & 128) != 0
	tFrame.Len = uint64(head[1] & 127)

	// read the extended lenght fields, if required
	var tLenBuffer [8]byte
	var tLenBufPtr []byte
	switch tFrame.Len {
	case 126:
		tLenBufPtr = tLenBuffer[6:]
	case 127:
		tLenBufPtr = tLenBuffer[:]
	default:
		tLenBufPtr = nil
	}
	if tLenBufPtr != nil {
		n, err = conn.Read(tLenBufPtr)
		if err != nil {
			return nil, err
		}
		if n != len(tLenBufPtr) {
			return nil, errors.New("incomplete length field")
		}
		tFrame.Len = binary.BigEndian.Uint64(tLenBuffer[:])
	}

	if tFrame.Masked {
		n, err = conn.Read(tFrame.Mask[:])
		if err != nil {
			return nil, err
		}
		if n != len(tFrame.Mask) {
			return nil, errors.New("incomplete mask")
		}
	}

	// read the payload
	tFrame.Payload = make([]byte, tFrame.Len, tFrame.Len)
	len := uint64(0)
	waiter := sync.WaitGroup{}
	for len != tFrame.Len {
		n, err := conn.Read(tFrame.Payload[len:])
		if err != nil {
			return nil, err
		}
		if tFrame.Masked {
			waiter.Add(1)
			go func(m, n uint64) {
				for i := m; i < m+n; i++ {
					tFrame.Payload[i] = tFrame.Payload[i] ^ tFrame.Mask[i%4]
				}
				waiter.Done()
			}(len, uint64(n))
		}
		len += uint64(n)
	}
	waiter.Wait()
	return tFrame, nil
}

// Write this Frame to the wire.
func (f *Frame) WriteTo(conn net.Conn) error {
	var head [2]byte
	if f.Fin {
		head[0] |= 1 << 7
	}
	head[0] |= uint8(f.Opcode) & 15
	if f.Masked {
		head[1] |= 1 << 7
	}
	var lbuf []byte = nil
	if len(f.Payload) < 126 {
		head[1] |= byte(len(f.Payload) & 127)
	} else if len(f.Payload) < 65536 {
		head[1] |= 126
		lbuf = make([]byte, 2, 2)
		binary.BigEndian.PutUint16(lbuf, uint16(len(f.Payload)))
	} else {
		head[1] |= 127
		lbuf = make([]byte, 8, 8)
		// only 63 of 64 bits can be used - no problem for us:
		// maximum slice size in go is max(int) so we never ever will reach bit 63 and we do not have to reset that bit.
		binary.BigEndian.PutUint64(lbuf, uint64(len(f.Payload)))
	}
	n, err := conn.Write(head[:])
	if err != nil {
		return err
	}
	if n != len(head) {
		return errors.New("partial head write")
	}
	if lbuf != nil {
		conn.Write(lbuf)
	}
	if f.Masked {
		conn.Write(f.Mask[:])
	}
	l := 0
	for l < len(f.Payload) {
		n, err = conn.Write(f.Payload[l:])
		if err != nil {
			return err
		}
		l += n
	}
	return nil
}