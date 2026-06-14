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

type TCPPeer struct {
	net.Conn

	streamStartCh chan struct{}
	streamDoneCh  chan struct{}

	iv            []byte
	peerIV        []byte
	secretKey     []byte
	peerPublicKey []byte

	outbound bool
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		Conn:          conn,
		outbound:      outbound,
		streamStartCh: make(chan struct{}),
		streamDoneCh:  make(chan struct{}),
	}
}

func (peer *TCPPeer) Send(t byte, r io.Reader, size int64) error {
	_, err := peer.Write([]byte{t})
	if err != nil {
		return err
	}

	if r == nil || size == 0 {
		return nil
	}

	if t == IncomingMessage {
		err := binary.Write(peer, binary.LittleEndian, size)
		if err != nil {
			return err
		}
	}

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
	if err != nil {
		return 0, err
	}

	_, err = peer.Conn.Write(cb.Bytes())
	if err != nil {
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
	if err != nil {
		return 0, err
	}

	n, err = CopyDecrypt(peer.secretKey, buf, bytes.NewReader(cb[:n]), peer.peerIV)
	if err != nil {
		return 0, err
	}
	copy(b, buf.Bytes())

	return buf.Len(), nil
}

func (peer *TCPPeer) ReadStream(size int64) io.Reader {
	select {
	case <-peer.streamStartCh:
	case <-time.After(streamTimeout):
	}
	return io.LimitReader(peer, size)
}

func (peer *TCPPeer) CloseStream() {
	select {
	case peer.streamDoneCh <- struct{}{}:
	case <-time.After(streamTimeout):
	}
}

func (peer *TCPPeer) ConsumeStreamStart() {
	select {
	case <-peer.streamStartCh:
	case <-time.After(streamTimeout):
	}
}

// TCPTransportOpts configures a TCPTransport.
type TCPTransportOpts struct {
	ListenAddr        string
	HandshakeFunc     HandshakeFunc
	Decoder           Decoder
	OnPeer            func(Peer) error
	ReconnectAttempts int
	ReconnectBackoff  time.Duration
}

// TCPTransport listens and dials TCP connections with encrypted peer channels.
type TCPTransport struct {
	TCPTransportOpts
	listener net.Listener
	rpcch    chan RPC
}

// NewTCPTransport creates a TCPTransport with defaults for nil HandshakeFunc and Decoder.
func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {
	if opts.HandshakeFunc == nil {
		opts.HandshakeFunc = NOPHandshakeFunc
	}
	if opts.Decoder == nil {
		opts.Decoder = &DefaultDecoder{}
	}
	return &TCPTransport{
		TCPTransportOpts: opts,
		rpcch:            make(chan RPC),
	}
}

func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcch
}

func (t *TCPTransport) Close() error {
	return t.listener.Close()
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error
	t.listener, err = net.Listen("tcp", t.ListenAddr)
	if err != nil {
		return err
	}

	go t.startAcceptConnLoop()

	log.Printf("TCP transport listening on: %s\n", t.ListenAddr)

	return nil
}

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
		if err != nil {
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

func (t *TCPTransport) initPeer(conn net.Conn, outbound bool) (*TCPPeer, error) {
	peer := NewTCPPeer(conn, outbound)

	secretKey, iv, peerIV, peerPublicKey, err := t.HandshakeFunc(conn)
	if err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	peer.secretKey = secretKey
	peer.peerIV = peerIV
	peer.iv = iv
	peer.peerPublicKey = peerPublicKey

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
