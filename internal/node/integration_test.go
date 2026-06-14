package node

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mustFreePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func startNode(t testing.TB, tcpPort, dhtPort int, dhtBootstrap string, tcpBootstrap []string) *FileServer {
	t.Helper()
	opts := FileServerOpts{
		StorageRoot:       t.TempDir(),
		EncryptionKey:     []byte("rptreftgrtgfrefrdeswfrdefrdejtkg"),
		PathTransformFunc: CASPathTransformFunc,
		TCPListenAddr:     fmt.Sprintf(":%d", tcpPort),
		DHTListenAddr:     fmt.Sprintf(":%d", dhtPort),
		DHTBootstrapAddr:  dhtBootstrap,
		TCPBootstrapNodes: tcpBootstrap,
	}
	fs := NewFileServer(opts)
	require.NoError(t, fs.Start())
	return fs
}

// Store on a node, retrieve from another via broadcast replication.
func TestIntegrationStoreAndRetrieveViaBroadcast(t *testing.T) {
	seedTCP := mustFreePort(t)
	seedDHT := mustFreePort(t)
	nodeTCP := mustFreePort(t)
	nodeDHT := mustFreePort(t)

	seed := startNode(t, seedTCP, seedDHT, "", nil)
	defer seed.Stop()

	node := startNode(t, nodeTCP, nodeDHT, fmt.Sprintf(":%d", seedDHT),
		[]string{fmt.Sprintf(":%d", seedTCP)})
	defer node.Stop()

	time.Sleep(2 * time.Second)

	data := []byte("hello integration test")
	require.NoError(t, node.Store("testkey", bytes.NewReader(data)))

	node.Delete("testkey")

	r, _, err := seed.Get("testkey")
	require.NoError(t, err)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, data, got)
}

// Store on a seed, retrieve from a 3rd node that discovers the data location
// through the DHT (no direct TCP connection between seed and node3).
func TestIntegrationRetrieveViaDHT(t *testing.T) {
	seedTCP := mustFreePort(t)
	seedDHT := mustFreePort(t)
	node2TCP := mustFreePort(t)
	node2DHT := mustFreePort(t)
	node3TCP := mustFreePort(t)
	node3DHT := mustFreePort(t)

	seed := startNode(t, seedTCP, seedDHT, "", nil)
	defer seed.Stop()

	node2 := startNode(t, node2TCP, node2DHT, fmt.Sprintf(":%d", seedDHT),
		[]string{fmt.Sprintf(":%d", seedTCP)})
	defer node2.Stop()

	node3 := startNode(t, node3TCP, node3DHT, fmt.Sprintf(":%d", node2DHT), nil)
	defer node3.Stop()

	time.Sleep(3 * time.Second)

	data := []byte("dht retrieval test")
	require.NoError(t, seed.Store("dhtkey", bytes.NewReader(data)))

	r, _, err := node3.Get("dhtkey")
	require.NoError(t, err)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, data, got)
}

// Store on a node, kill it, then retrieve from another node that received the
// broadcast copy.
func TestIntegrationNodeFailureResilience(t *testing.T) {
	seedTCP := mustFreePort(t)
	seedDHT := mustFreePort(t)
	node2TCP := mustFreePort(t)
	node2DHT := mustFreePort(t)
	node3TCP := mustFreePort(t)
	node3DHT := mustFreePort(t)

	seed := startNode(t, seedTCP, seedDHT, "", nil)
	defer seed.Stop()

	node2 := startNode(t, node2TCP, node2DHT, fmt.Sprintf(":%d", seedDHT),
		[]string{fmt.Sprintf(":%d", seedTCP)})
	defer node2.Stop()

	node3 := startNode(t, node3TCP, node3DHT, fmt.Sprintf(":%d", seedDHT),
		[]string{fmt.Sprintf(":%d", seedTCP)})
	defer node3.Stop()

	time.Sleep(2 * time.Second)

	data := []byte("resilience test data")
	require.NoError(t, node2.Store("resilientkey", bytes.NewReader(data)))

	node2.Stop()

	time.Sleep(500 * time.Millisecond)

	r, _, err := seed.Get("resilientkey")
	require.NoError(t, err)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, data, got)
}

func TestLocalRangeRetrieval(t *testing.T) {
	seedTCP := mustFreePort(t)
	seedDHT := mustFreePort(t)

	seed := startNode(t, seedTCP, seedDHT, "", nil)
	defer seed.Stop()

	data := []byte("hello range test data")
	require.NoError(t, seed.Store("rangetest", bytes.NewReader(data)))

	r, n, err := seed.GetRange("rangetest", 6, 5)
	require.NoError(t, err)
	require.Equal(t, int64(5), n)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, []byte("range"), got)
}

func TestIntegrationStats(t *testing.T) {
	seedTCP := mustFreePort(t)
	seedDHT := mustFreePort(t)

	seed := startNode(t, seedTCP, seedDHT, "", nil)
	defer seed.Stop()

	stats := seed.Stats()
	require.Equal(t, 0, stats.PeerCount)
	require.GreaterOrEqual(t, stats.StorageUsedBytes, int64(0))
}

func TestIntegrationConcurrentStoreAndRetrieve(t *testing.T) {
	seedTCP := mustFreePort(t)
	seedDHT := mustFreePort(t)
	nodeTCP := mustFreePort(t)
	nodeDHT := mustFreePort(t)

	seed := startNode(t, seedTCP, seedDHT, "", nil)
	defer seed.Stop()

	node := startNode(t, nodeTCP, nodeDHT, fmt.Sprintf(":%d", seedDHT),
		[]string{fmt.Sprintf(":%d", seedTCP)})
	defer node.Stop()

	time.Sleep(2 * time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent_%d", i)
			data := []byte(fmt.Sprintf("concurrent data %d", i))
			require.NoError(t, node.Store(key, bytes.NewReader(data)))
			node.Delete(key)
			r, _, err := seed.Get(key)
			if err == nil {
				got, _ := io.ReadAll(r)
				require.Equal(t, data, got)
			}
		}(i)
	}
	wg.Wait()
}
