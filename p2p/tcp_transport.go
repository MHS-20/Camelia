package p2p

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const streamTimeout = 30 * time.Second

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// TCPPeer represent the remote node over a TCP established connection
type TCPPeer struct {
    net.Conn

    // streamStartCh is used by the read loop to signal the start of a stream
    streamStartCh chan struct{}
    // streamDoneCh is used by the consumer to signal the end of a stream
    streamDoneCh chan struct{}

    // secretKey will be the key to used in encryption and decryption of traffic 
    iv []byte
    peerIV []byte
    secretKey []byte

    // if we initiate the connection ==> outbound == false
    // if we accept and retrieve a connection ==> outbound == true
    outbound bool
}

// Write function takes type byte, io.Reader and that reader's size first send the byte type 
// so remote peer will be ready for steam or normal message based on this type 
// if t == IncomingMessage --> then content length will be transferred and then the actual message bytes will be sent
// if t == IncomingStream --> then directly the stream bytes will be sent
func (peer *TCPPeer) Send(t byte, r io.Reader, size int64) error {
    // Send incoming data type to remote peer
    _, err := peer.Write([]byte{t})
    if err != nil {
        return err
    }

    if r == nil || size == 0 { return nil }

    if t == IncomingMessage {
        err := binary.Write(peer, binary.LittleEndian, size)
        if err != nil {
            return err
        }
    }
    
    // Send message or stream bytes to remote peer
    _, err = io.Copy(peer, r)
    if err != nil {
        return err
    }

    return nil
}

func (peer *TCPPeer) Write(b []byte) (n int, err error) {
    cb := bufPool.Get().(*bytes.Buffer)
    cb.Reset()
    defer bufPool.Put(cb)

    n, err = CopyEncrypt(peer.secretKey, cb, bytes.NewReader(b), peer.iv)
    if(err != nil) {
        return 0, err
    }

    _, err = peer.Conn.Write(cb.Bytes())
    if(err != nil) {
        return 0, err
    }
    
    return len(b), nil
}

func (peer *TCPPeer) Read(b []byte) (n int, err error) {
    buf := bufPool.Get().(*bytes.Buffer)
    buf.Reset()
    defer bufPool.Put(buf)

    cb := make([]byte, cap(b)) 

    n, err = peer.Conn.Read(cb)
    if(err != nil) {
        return 0, err
    }

    n, err = CopyDecrypt(peer.secretKey, buf, bytes.NewReader(cb[:n]), peer.peerIV)
    if(err != nil) {
        return 0, err
    }
    copy(b, buf.Bytes())

    return buf.Len(), nil
}

// ReadStream function implements Peer interface
// it will be used when user want to use a specific size of stream
func (peer *TCPPeer) ReadStream(size int64) io.Reader {
	select {
	case <-peer.streamStartCh:
	case <-time.After(streamTimeout):
	}
	return io.LimitReader(peer, size)
}

// CloseStream function implements Peer interface
// it is used to continue the read loop of messages after reading the stream of previous message from peer connection reader
func (peer *TCPPeer) CloseStream() {
	select {
	case peer.streamDoneCh <- struct{}{}:
	case <-time.After(streamTimeout):
	}
}

// ConsumeStreamStart reads and discards a pending stream start signal
// without reading any data. Used when a peer indicates no data follows.
func (peer *TCPPeer) ConsumeStreamStart() {
	select {
	case <-peer.streamStartCh:
	case <-time.After(streamTimeout):
	}
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		Conn: conn,
		outbound: outbound,
		streamStartCh: make(chan struct{}),
		streamDoneCh: make(chan struct{}),
	}
}

type TCPTransportOpts struct {
    ListenAddr string
    HandshakeFunc HandshakeFunc
    Decoder Decoder
    OnPeer func(Peer) error
    ReconnectAttempts int
    ReconnectBackoff time.Duration
}

type TCPTransport struct {
    TCPTransportOpts
    listener net.Listener
    rpcch chan RPC
}

func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {
    if opts.HandshakeFunc == nil {
        opts.HandshakeFunc = NOPHandshakeFunc
    }
    if  opts.Decoder == nil {
        opts.Decoder = &DefaultDecoder{}
    }
    return &TCPTransport{
        TCPTransportOpts: opts,
        rpcch: make(chan RPC),
    }
}

// Consume implements the transport interface, which will return a read only channel
// for reading incoming messages received from another peer in the network
func (t *TCPTransport) Consume() <-chan RPC {
    return t.rpcch
}

// Close implements the Transport interface
func (t *TCPTransport) Close() error {
    return t.listener.Close()
}

func (t *TCPTransport) ListenAndAccept() error {
    var err error
    t.listener, err = net.Listen("tcp", t.ListenAddr)
    if err!=nil {
        return err
    }

    go t.startAcceptConnLoop() 

    log.Printf("TCP transport listening on: %s\n", t.ListenAddr)

    return nil
}

// Dial implements the Transport interface
// It connects, performs the handshake, and registers the peer synchronously,
// then starts the read loop in a background goroutine.
// If ReconnectAttempts > 0, it will automatically retry dropped outbound connections.
func (t *TCPTransport) Dial(addr string) error {
	return t.dialWithRetry(addr, 0)
}

func (t *TCPTransport) dialWithRetry(addr string, attempt int) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	peer, err := t.initPeer(conn, true)
	if err != nil {
		conn.Close()
		return err
	}

	go t.readLoopWithRetry(peer, conn, addr, attempt)
	return nil
}

func (t *TCPTransport) readLoopWithRetry(peer *TCPPeer, conn net.Conn, addr string, attempt int) {
	t.readLoop(peer, conn)

	if t.ReconnectAttempts > 0 && attempt < t.ReconnectAttempts {
		backoff := t.ReconnectBackoff
		if backoff == 0 {
			backoff = time.Second
		}
		backoff <<= attempt
		log.Printf("reconnecting to %s in %v (attempt %d/%d)", addr, backoff, attempt+1, t.ReconnectAttempts)
		time.Sleep(backoff)
		if err := t.dialWithRetry(addr, attempt+1); err != nil {
			log.Printf("reconnect to %s failed: %v", addr, err)
		}
	}
}

func (t *TCPTransport) startAcceptConnLoop() {
	for {
		conn, err := t.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err!=nil {
			fmt.Printf("TCP accept error: %s\n", err)
		}

		go func() {
			peer, err := t.initPeer(conn, false)
			if err != nil {
				fmt.Printf("peer init error: %s\n", err)
				conn.Close()
				return
			}
			t.readLoop(peer, conn)
		}()
	}
}

// initPeer performs the handshake and calls OnPeer synchronously.
func (t *TCPTransport) initPeer(conn net.Conn, outbound bool) (*TCPPeer, error) {
	peer := NewTCPPeer(conn, outbound)

	secretKey, iv, peerIV, err := t.HandshakeFunc(conn)
	if err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	peer.secretKey = secretKey
	peer.peerIV = peerIV
	peer.iv = iv

	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			return nil, fmt.Errorf("OnPeer: %w", err)
		}
	}

	return peer, nil
}

func (t *TCPTransport) readLoop(peer *TCPPeer, conn net.Conn) {
	defer func() {
		fmt.Printf("dropping peer connection: %s\n", conn.RemoteAddr())
		conn.Close()
	}()

	for {
		rpc := RPC{}
		if err := t.Decoder.Decode(peer, &rpc); err != nil {
			if ne := net.Error(nil); errors.As(err, &ne) && (ne.Timeout() || ne.Temporary()) {
				continue
			}
			return
		}

		rpc.From = conn.RemoteAddr().String()

		if rpc.Stream {
			select {
			case peer.streamStartCh <- struct{}{}:
			case <-time.After(streamTimeout):
				return
			}
			select {
			case <-peer.streamDoneCh:
			case <-time.After(streamTimeout):
				return
			}
			continue
		}

		t.rpcch <- rpc
	}
}
