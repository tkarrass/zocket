package zocket

// Addr represents a (the) WebSocket end point address.
// Since WebSocket connections are upgraded HTTP Connections,
//the Listener always has a dummy address
type Addr struct {
	network string
	address string
}

// Implement the net.Addr interface

func (a Addr) Network() string { return a.network }
func (a Addr) String() string  { return a.address }

// Some default Addr for internal use
var addr = Addr{
	network: "ws",
	address: "[WebSocket]",
}
