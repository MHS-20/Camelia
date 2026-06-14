package node

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/MHS-20/Kademlia/kademlia"
	"github.com/chiragsoni81245/foreverstore/p2p"
)

type Message struct {
    Payload any
}

type MessageGetFile struct {
    Key string
}

type MessageStoreFile struct {
    Key string
    Size int64
}

type FileServerOpts struct {
    StorageRoot string
    PathTransformFunc PathTransformFunc
    EncryptionKey []byte
    TCPListenAddr string
    DHTListenAddr string
    TCPBootstrapNodes []string
    DHTBootstrapAddr string
}

type FileServer struct {
    FileServerOpts

    dhtNode      *kademlia.Node
    dhtTransport *kademlia.UDPTransport
    tcpTransport *p2p.TCPTransport

    peerLock sync.Mutex
    peers map[string]p2p.Peer

    store *Store
    quitch chan struct{}
    stopOnce sync.Once

    tofuStore *TofuStore
}

func NewFileServer(opts FileServerOpts) *FileServer {
    storeOpts := StoreOpts{
        Root: opts.StorageRoot,
        PathTransformFunc: opts.PathTransformFunc,
    }

    tofuStore, err := NewTofuStore(opts.StorageRoot)
    if err != nil {
        log.Printf("failed to initialise TOFU store: %v", err)
    }

    tcpOpts := p2p.TCPTransportOpts{
        ListenAddr: opts.TCPListenAddr,
        HandshakeFunc: func(conn net.Conn) ([]byte, []byte, []byte, []byte, error) {
            secretKey, iv, peerIV, peerPublicKey, err := p2p.DiffieHellmanHandshake(conn)
            if err != nil {
                return nil, nil, nil, nil, err
            }
            if tofuStore != nil {
                peerID := conn.RemoteAddr().String()
                if err := tofuStore.CheckOrPin(peerID, peerPublicKey); err != nil {
                    return nil, nil, nil, nil, err
                }
            }
            return secretKey, iv, peerIV, peerPublicKey, nil
        },
        Decoder: &p2p.DefaultDecoder{},
    }
    tcpTransport := p2p.NewTCPTransport(tcpOpts)

    selfContact := kademlia.Contact{
        ID:   kademlia.RandomNodeID(),
        Addr: &net.UDPAddr{IP: net.IPv4zero, Port: addrPort(opts.DHTListenAddr)},
    }
    dhtTrans := kademlia.NewUDPTransport()
    dhtMemStore := kademlia.NewMemoryStore()
    dhtNode := kademlia.NewNode(selfContact, dhtTrans, dhtMemStore)

    fs := &FileServer{
        FileServerOpts: opts,
        store: NewStore(storeOpts),
        tcpTransport: tcpTransport,
        dhtNode: dhtNode,
        dhtTransport: dhtTrans,
        quitch: make(chan struct{}),
        peers: make(map[string]p2p.Peer),
        tofuStore: tofuStore,
    }
    tcpTransport.OnPeer = fs.OnPeer

    return fs
}

const maxKeyLength = 1024

func validateKey(key string) error {
    if key == "" {
        return fmt.Errorf("key must not be empty")
    }
    if len(key) > maxKeyLength {
        return fmt.Errorf("key length %d exceeds maximum %d", len(key), maxKeyLength)
    }
    return nil
}

func (fs *FileServer) Delete(key string) error {
    if err := validateKey(key); err != nil {
        return err
    }
    return fs.store.Delete(key)
}

func addrPort(addr string) int {
    _, port, _ := net.SplitHostPort(addr)
    var p int
    fmt.Sscanf(port, "%d", &p)
    return p
}

func (fs *FileServer) getPeer(addr string) (p2p.Peer, error) {
    fs.peerLock.Lock()
    defer fs.peerLock.Unlock()

    peer, ok := fs.peers[addr]
    if !ok {
        return nil, fmt.Errorf("peer not found in peer list")
    }

    return peer, nil
}

func (fs *FileServer) dialPeer(addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("invalid peer address %q: %w", addr, err)
	}
	fs.peerLock.Lock()
	_, exists := fs.peers[addr]
	fs.peerLock.Unlock()
	if exists {
		return nil
	}
	return fs.tcpTransport.Dial(addr)
}

func (fs *FileServer) bootstrapTCPNetwork() error {
    for _, addr := range fs.TCPBootstrapNodes {
        log.Printf("connecting to TCP bootstrap: %s", addr)
        if err := fs.dialPeer(addr); err != nil {
            log.Printf("TCP bootstrap dial %s: %v", addr, err)
        }
    }
    return nil
}

func (fs *FileServer) loop() {
	defer func(){
		log.Printf("file server stopped due to user quit action")
		fs.tcpTransport.Close()
	}()
	for {
		select {
		case rpc := <-fs.tcpTransport.Consume():
			var msg Message;
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&msg); err != nil {
				log.Printf("failed to decode message from %s: %v", rpc.From, err)
				continue
			}

			if err := fs.handleMessage(&rpc, &msg); err != nil {
				log.Printf("failed to handle message from %s: %v", rpc.From, err)
				continue
			}
		case <-fs.quitch:
			return
		}
	}
}

func (fs *FileServer) handleMessage(rpc *p2p.RPC, msg *Message) error {
    switch payload := msg.Payload.(type) {
    case *MessageStoreFile:
        return fs.handleStoreFileMessage(rpc, payload)
    case *MessageGetFile:
        return fs.handleGetFileMessage(rpc, payload)
    }
    return nil
}

func (fs *FileServer) handleStoreFileMessage(rpc *p2p.RPC, msgPayload *MessageStoreFile) error {
    log.Printf("store message received: %+v\n", msgPayload)

    peer, err := fs.getPeer(rpc.From)
    if err != nil {
        return err
    }

    if _, err := fs.store.Write(msgPayload.Key, peer.ReadStream(msgPayload.Size)); err != nil {
        return err
    }

    peer.CloseStream()

    return nil
}

func (fs *FileServer) handleGetFileMessage(rpc *p2p.RPC, msgPayload *MessageGetFile) error {
	log.Printf("get message received: %+v\n", msgPayload)

	var (
		r    io.ReadCloser
		size int64
		err  error
	)

	if !fs.store.Has(msgPayload.Key) {
		log.Printf("file requested via peer %s not found", rpc.From)
	} else {
		r, size, err = fs.store.Read(msgPayload.Key)
		if err != nil {
			return err
		}
		defer r.Close()
	}

	peer, err := fs.getPeer(rpc.From)
	if err != nil {
		return err
	}

	if err := peer.Send(p2p.IncomingStream, nil, 0); err != nil {
		return err
	}

	if err := binary.Write(peer, binary.LittleEndian, size); err != nil {
		return err
	}

	if size != 0 && r != nil {
		_, err = io.Copy(peer, r)
	}

	return err
}

func (fs *FileServer) broadcast(msg *Message, r io.Reader) error {
    msgBuf := new(bytes.Buffer)
    if err := gob.NewEncoder(msgBuf).Encode(msg); err != nil {
        return err
    }

    var streamData []byte
    if r != nil {
        var err error
        streamData, err = io.ReadAll(r)
        if err != nil {
            return err
        }
    }

    var wg sync.WaitGroup
    for _, peer := range fs.peers {
        wg.Add(1)
        go func(peer p2p.Peer) {
            defer wg.Done()
            if err := peer.Send(p2p.IncomingMessage, bytes.NewReader(msgBuf.Bytes()), int64(msgBuf.Len())); err != nil {
                log.Printf("error in sending message to peer %s: %v", peer.RemoteAddr().String(), err)
                return
            }
            switch msg.Payload.(type) {
            case *MessageStoreFile:
                if len(streamData) > 0 {
                    if err := peer.Send(p2p.IncomingStream, bytes.NewReader(streamData), int64(len(streamData))); err != nil {
                        log.Printf("error in sending stream to peer %s: %v", peer.RemoteAddr().String(), err)
                    }
                }
            }
        }(peer)
    }
    wg.Wait()
    return nil
}

func (fs *FileServer) queryTCPPeers(key string) (io.ReadCloser, int64, error) {
    getFileMsg := &Message{
        Payload: &MessageGetFile{
            Key: key,
        },
    }
    getFileMsgBuf := new(bytes.Buffer)
    if err := gob.NewEncoder(getFileMsgBuf).Encode(getFileMsg); err != nil {
        return nil, 0, err
    }

    fs.peerLock.Lock()
    peers := make([]p2p.Peer, 0, len(fs.peers))
    for _, peer := range fs.peers {
        peers = append(peers, peer)
    }
    fs.peerLock.Unlock()

    if len(peers) == 0 {
        return nil, 0, fmt.Errorf("no peers to query")
    }

    type queryResult struct {
        r    io.ReadCloser
        size int64
        err  error
    }

    resultCh := make(chan queryResult, len(peers))
    var wg sync.WaitGroup

    msgBytes := getFileMsgBuf.Bytes()

    for _, peer := range peers {
        wg.Add(1)
        go func(peer p2p.Peer) {
            defer wg.Done()

            if err := peer.Send(p2p.IncomingMessage, bytes.NewReader(msgBytes), int64(len(msgBytes))); err != nil {
                return
            }

            var fileSize int64
            if err := binary.Read(peer, binary.LittleEndian, &fileSize); err != nil {
                return
            }

            if fileSize == 0 {
                peer.ConsumeStreamStart()
                peer.CloseStream()
                return
            }

            if _, err := fs.store.Write(key, peer.ReadStream(fileSize)); err != nil {
                return
            }
            peer.CloseStream()

            f, size, err := fs.store.Read(key)
            resultCh <- queryResult{f, size, err}
        }(peer)
    }

    go func() {
        wg.Wait()
        close(resultCh)
    }()

    var firstErr error
    for res := range resultCh {
        if res.err == nil {
            return res.r, res.size, nil
        }
        if firstErr == nil {
            firstErr = res.err
        }
    }

    if firstErr == nil {
        firstErr = fmt.Errorf("file not found on any TCP peer")
    }
    return nil, 0, firstErr
}

func (fs *FileServer) Get(key string) (f io.Reader, size int64, err error) {
    if err := validateKey(key); err != nil {
        return nil, 0, err
    }
    if fs.store.Has(key) {
        f, size, err = fs.store.Read(key)
        if err != nil {
            return nil, 0, err
        }
    } else {
        log.Printf("file not found locally, checking DHT")

        keyNodeID := kademlia.NewNodeID([]byte(key))

        val, closest, err := fs.dhtNode.FindValue(keyNodeID)
        if err != nil {
            log.Printf("DHT find value: %v", err)
        }

        if val != nil {
            tcpAddr := string(val)
            if tcpAddr != fs.TCPListenAddr {
                log.Printf("DHT advertisement found at %s, dialing via TCP", tcpAddr)
                    if err := fs.dialPeer(tcpAddr); err != nil {
                        log.Printf("dial %s: %v", tcpAddr, err)
                    } else {
                        f, size, err = fs.queryTCPPeers(key)
                        if err == nil {
                            goto decrypt
                        }
                        log.Printf("query via DHT peer failed: %v", err)
                    }
            } else {
                log.Printf("DHT advertisement points to self, skipping")
            }
        }

        if len(closest) > 0 {
            log.Printf("trying %d closest DHT nodes", len(closest))
            for _, c := range closest {
                nodeID := kademlia.NewNodeID([]byte(c.ID.String()))
                val, _, err := fs.dhtNode.FindValue(nodeID)
                if err != nil || val == nil {
                    log.Printf("no TCP address advertised for node %s", c.ID)
                    continue
                }
                tcpAddr := string(val)
                if tcpAddr == fs.TCPListenAddr {
                    continue
                }
                log.Printf("trying closest node at %s", tcpAddr)
                if err := fs.dialPeer(tcpAddr); err != nil {
                    log.Printf("dial %s: %v", tcpAddr, err)
                    continue
                }
                f, size, err = fs.queryTCPPeers(key)
                if err == nil {
                    goto decrypt
                }
            }
        }

        log.Printf("DHT lookup failed, checking existing TCP peers")
        f, size, err = fs.queryTCPPeers(key)
        if err != nil {
            return nil, 0, err
        }
    }

decrypt:
	decryptedBuf := new(bytes.Buffer)
	decryptedBufSize, err := p2p.CopyDecrypt(fs.EncryptionKey, decryptedBuf, f, nil)
	if err != nil {
		return nil, 0, err
	}
	if closer, ok := f.(io.Closer); ok {
		closer.Close()
	}
	return decryptedBuf, int64(decryptedBufSize), nil
}

func (fs *FileServer) Store(key string, r io.Reader) error {
    if err := validateKey(key); err != nil {
        return err
    }
    encryptedBuf := new(bytes.Buffer)
    p2p.CopyEncrypt(fs.EncryptionKey, encryptedBuf, r, nil)

    buf := new(bytes.Buffer)
    tee := io.TeeReader(encryptedBuf, buf)

    size, err := fs.store.Write(key, tee)
    if err != nil {
        return err
    }

    msg := &Message{
        Payload: &MessageStoreFile{
            Key: key,
            Size: size,
        },
    }

    if err := fs.broadcast(msg, buf); err != nil {
        log.Printf("broadcast error: %v", err)
    }

    keyNodeID := kademlia.NewNodeID([]byte(key))
    advertisement := []byte(fs.TCPListenAddr)
    if err := fs.dhtNode.Store(keyNodeID, advertisement); err != nil {
        log.Printf("DHT store advertisement: %v", err)
    }

    return nil
}

func (fs *FileServer) OnPeer(peer p2p.Peer) error{
    fs.peerLock.Lock()
    defer fs.peerLock.Unlock()

    fs.peers[peer.RemoteAddr().String()] = peer
    log.Printf("connected with remote %s", peer.RemoteAddr())
    return nil
}

func (fs *FileServer) Start() error{
    if err := fs.dhtTransport.Listen(fs.DHTListenAddr); err != nil {
        return fmt.Errorf("DHT listen: %w", err)
    }
    fs.dhtNode.Run()

    if err := fs.tcpTransport.ListenAndAccept(); err != nil {
        return err
    }

    if err := fs.bootstrapDHT(); err != nil {
        log.Println(err)
    }

    if err := fs.bootstrapTCPNetwork(); err != nil {
        log.Println(err)
    }

    go fs.loop()

    return nil
}

func (fs *FileServer) bootstrapDHT() error {
    if fs.DHTBootstrapAddr == "" {
        return nil
    }

    addr, err := net.ResolveUDPAddr("udp", fs.DHTBootstrapAddr)
    if err != nil {
        return fmt.Errorf("resolve DHT bootstrap addr: %w", err)
    }

    bootstrapContact := kademlia.Contact{Addr: addr}
    if err := fs.dhtNode.Bootstrap(bootstrapContact); err != nil {
        return fmt.Errorf("DHT bootstrap: %w", err)
    }

    myID := fs.dhtNode.Self().ID
    if err := fs.dhtNode.Store(myID, []byte(fs.TCPListenAddr)); err != nil {
        log.Printf("DHT advertise self: %v", err)
    }

    return nil
}

func (fs *FileServer) Stop() {
    fs.stopOnce.Do(func() {
        close(fs.quitch)
        fs.dhtNode.Stop()
    })
}

func init() {
    gob.Register(&MessageGetFile{})
    gob.Register(&MessageStoreFile{})
}
