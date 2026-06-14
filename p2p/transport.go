package p2p

import (
	"io"
	"net"
)

// Peer represents a remote node that can send and receive messages and streams.
type Peer interface {
	net.Conn
	Send(t byte, r io.Reader, size int64) error
	CloseStream()
	ReadStream(size int64) io.Reader
	ConsumeStreamStart()
}

// Transport handles network communication between nodes.
type Transport interface {
	ListenAndAccept() error
	Dial(string) error
	Consume() <-chan RPC
	Close() error
}
