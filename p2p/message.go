package p2p

const (
	IncomingMessage = 0x1
	IncomingStream  = 0x2
)

// RPC represents a parsed message or stream signal received from a peer.
type RPC struct {
    From string
    Size int64
    Stream bool
    Payload []byte
}
