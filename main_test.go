package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/chiragsoni81245/foreverstore/internal/node"
)

func TestMainDemo(t *testing.T) {
	tcpAddr := ":0"
	dhtAddr := ":0"

	opts := node.FileServerOpts{
		StorageRoot:       t.TempDir(),
		EncryptionKey:     []byte("rptreftgrtgfrefrdeswfrdefrdejtkg"),
		PathTransformFunc: node.CASPathTransformFunc,
		TCPListenAddr:     tcpAddr,
		DHTListenAddr:     dhtAddr,
	}

	fs := node.NewFileServer(opts)
	if err := fs.Start(); err != nil {
		t.Fatal(err)
	}
	defer fs.Stop()

	key := "testkey"
	data := []byte("test data for main_test")
	if err := fs.Store(key, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}

	r, _, err := fs.Get(key)
	if err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(data) {
		t.Fatalf("data mismatch: got %q, want %q", string(got), string(data))
	}
}
