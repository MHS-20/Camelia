package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chiragsoni81245/foreverstore/internal/node"
)

func main() {
	tcpAddr := env("TCP_ADDR", ":4000")
	dhtAddr := env("DHT_ADDR", ":9000")
	tcpBootstrap := env("TCP_BOOTSTRAP", "")
	dhtBootstrap := env("DHT_BOOTSTRAP", "")
	runTest, _ := strconv.ParseBool(env("RUN_TEST", "false"))

	encryptionKey := env("ENCRYPTION_KEY", "rptreftgrtgfrefrdeswfrdefrdejtkg")
	if encryptionKey == "" {
		log.Fatal("ENCRYPTION_KEY must be set to a 32-byte AES key")
	}
	if len(encryptionKey) != 32 {
		log.Fatalf("ENCRYPTION_KEY must be exactly 32 bytes, got %d", len(encryptionKey))
	}

	opts := node.FileServerOpts{
		StorageRoot:       fmt.Sprintf("storage/%s_network", tcpAddr),
		EncryptionKey:     []byte(encryptionKey),
		PathTransformFunc: node.CASPathTransformFunc,
		TCPListenAddr:     tcpAddr,
		DHTListenAddr:     dhtAddr,
		DHTBootstrapAddr:  dhtBootstrap,
	}

	if tcpBootstrap != "" {
		opts.TCPBootstrapNodes = strings.Split(tcpBootstrap, ",")
	}

	fs := node.NewFileServer(opts)
	if err := fs.Start(); err != nil {
		log.Fatal(err)
	}

	httpAddr := env("HTTP_ADDR", "")
	if httpAddr != "" {
		srv := node.NewHTTPServer(fs, httpAddr)
		if err := srv.Start(); err != nil {
			log.Fatal(err)
		}
		defer srv.Stop()
	}

	if runTest {
		time.Sleep(2 * time.Second)
		runDemoTest(fs)
		fs.Stop()
		return
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("received signal %v, shutting down", sig)
	fs.Stop()
}

func runDemoTest(fs *node.FileServer) {
	key := "myprivatedata"

	data := []byte("hello from foreverstore at " + time.Now().String())
	tmpFile, err := os.CreateTemp("", "foreverstore-*")
	if err != nil {
		log.Fatalf("create temp file: %v", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		log.Fatalf("write temp file: %v", err)
	}
	tmpFile.Seek(0, 0)

	log.Printf("Storing %d bytes under key %q", len(data), key)
	if err := fs.Store(key, tmpFile); err != nil {
		log.Fatalf("store: %v", err)
	}
	tmpFile.Close()

	time.Sleep(1 * time.Second)

fs.Delete(key)
	log.Printf("Deleted local copy of %q, retrieving from network...", key)

	r, _, err := fs.Get(key)
	if err != nil {
		log.Fatalf("get: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		log.Fatalf("read: %v", err)
	}

	if string(out) != string(data) {
		log.Fatalf("data mismatch: got %q, want %q", string(out), string(data))
	}

	log.Printf("Test PASSED: retrieved %d bytes, content matches", len(out))
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
