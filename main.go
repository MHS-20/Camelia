package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

func main(){
    fs1 := makeserver(":4000", ":9000")
    fs1.Start()

    time.Sleep(1*time.Second)

    fs2Opts := FileServerOpts{
        StorageRoot: fmt.Sprintf("storage/%s_network", ":5000"),
        EncryptionKey: []byte("rptreftgrtgfrefrdeswfrdefrdejtkg"),
        PathTransformFunc: CASPathTransformFunc,
        TCPListenAddr: ":5000",
        DHTListenAddr: ":9001",
        TCPBootstrapNodes: []string{":4000"},
        DHTBootstrapAddr: ":9000",
    }
    fs2 := NewFileServer(fs2Opts)
    fs2.Start()
    time.Sleep(1*time.Second)

    key := "myprivatedata"
    f, err := os.Open("test_file")
    if err := fs2.Store(key, f); err != nil {
        log.Fatal(err)
    }
    f.Close()

    time.Sleep(1*time.Second)
    fs2.store.Delete(key)

    r, _, err := fs2.Get(key)
    if err != nil {
        log.Fatal(err)
    }

    f, err = os.Create("output_file")
    n, err := io.Copy(f, r)
    if err != nil {
        log.Fatal(err)
    }
    f.Close()

    log.Printf("Received %d bytes", n)

    select {}
}

func makeserver(tcpAddr, dhtAddr string) *FileServer {
    opts := FileServerOpts{
        StorageRoot: fmt.Sprintf("storage/%s_network", tcpAddr),
        EncryptionKey: []byte("rptreftgrtgfrefrdeswfrdefrdejtkg"),
        PathTransformFunc: CASPathTransformFunc,
        TCPListenAddr: tcpAddr,
        DHTListenAddr: dhtAddr,
    }
    return NewFileServer(opts)
}
