package main

import (
	"encoding/json"
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
	cfgPath := env("CONFIG", "config.json")
	cfg := loadConfig(cfgPath)

	defTCP := ":4000"
	defDHT := ":9000"
	defKey := "rptreftgrtgfrefrdeswfrdefrdejtkg"
	if cfg != nil {
		if cfg.TCPAddr != "" {
			defTCP = cfg.TCPAddr
		}
		if cfg.DHTAddr != "" {
			defDHT = cfg.DHTAddr
		}
		if cfg.EncryptionKey != "" {
			defKey = cfg.EncryptionKey
		}
	}

	tcpAddr := env("TCP_ADDR", defTCP)
	dhtAddr := env("DHT_ADDR", defDHT)
	tcpBootstrap := env("TCP_BOOTSTRAP", "")
	dhtBootstrap := env("DHT_BOOTSTRAP", "")
	runTest, _ := strconv.ParseBool(env("RUN_TEST", "false"))

	encryptionKey := env("ENCRYPTION_KEY", defKey)
	if encryptionKey == "" {
		log.Fatal("ENCRYPTION_KEY must be set to a 32-byte AES key")
	}
	if len(encryptionKey) != 32 {
		log.Fatalf("ENCRYPTION_KEY must be exactly 32 bytes, got %d", len(encryptionKey))
	}

	defStorageRoot := fmt.Sprintf("storage/%s_network", tcpAddr)
	if cfg != nil && cfg.StorageRoot != "" {
		defStorageRoot = cfg.StorageRoot
	}
	storageRoot := env("STORAGE_ROOT", defStorageRoot)

	opts := node.FileServerOpts{
		StorageRoot:       storageRoot,
		EncryptionKey:     []byte(encryptionKey),
		PathTransformFunc: node.CASPathTransformFunc,
		TCPListenAddr:     tcpAddr,
		DHTListenAddr:     dhtAddr,
		DHTBootstrapAddr:  dhtBootstrap,
	}

	if cfg != nil && cfg.MaxStorageMB > 0 {
		opts.MaxStorageBytes = cfg.MaxStorageMB * 1024 * 1024
	}

	if tcpBootstrap != "" {
		opts.TCPBootstrapNodes = strings.Split(tcpBootstrap, ",")
	}

	fs := node.NewFileServer(opts)
	if err := fs.Start(); err != nil {
		log.Fatal(err)
	}

	defHTTP := ""
	if cfg != nil {
		defHTTP = cfg.HTTPAddr
	}
	httpAddr := env("HTTP_ADDR", defHTTP)
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

type configFile struct {
	TCPAddr       string `json:"tcp_addr"`
	DHTAddr       string `json:"dht_addr"`
	TCPBootstrap  string `json:"tcp_bootstrap"`
	DHTBootstrap  string `json:"dht_bootstrap"`
	EncryptionKey string `json:"encryption_key"`
	HTTPAddr      string `json:"http_addr"`
	StorageRoot   string `json:"storage_root"`
	MaxStorageMB  int64  `json:"max_storage_mb"`
}

func loadConfig(path string) *configFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("failed to parse config file %s: %v", path, err)
		return nil
	}
	return &cfg
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
