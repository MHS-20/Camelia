package p2p

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPTransport(t *testing.T) {
	opts := TCPTransportOpts{
		ListenAddr:    ":0",
		HandshakeFunc: NOPHandshakeFunc,
		Decoder:       &DefaultDecoder{},
	}
	tr := NewTCPTransport(opts)
	require.NoError(t, tr.ListenAndAccept())
	defer tr.Close()

	addr := tr.listener.Addr().String()
	require.NoError(t, tr.Dial(addr))
	time.Sleep(100 * time.Millisecond)
}

func TestTCPTransportConcurrentDial(t *testing.T) {
	opts := TCPTransportOpts{
		ListenAddr:    ":0",
		HandshakeFunc: NOPHandshakeFunc,
		Decoder:       &DefaultDecoder{},
	}
	tr := NewTCPTransport(opts)
	require.NoError(t, tr.ListenAndAccept())
	defer tr.Close()

	addr := tr.listener.Addr().String()
	for i := 0; i < 5; i++ {
		require.NoError(t, tr.Dial(addr))
	}
	time.Sleep(200 * time.Millisecond)
}

func TestTCPTransportListenAddr(t *testing.T) {
	opts := TCPTransportOpts{
		ListenAddr:    ":0",
		HandshakeFunc: NOPHandshakeFunc,
		Decoder:       &DefaultDecoder{},
	}
	tr := NewTCPTransport(opts)
	require.NoError(t, tr.ListenAndAccept())
	defer tr.Close()

	assert.NotEqual(t, ":0", tr.listener.Addr().String())
	assert.Contains(t, tr.listener.Addr().String(), ":")
}
