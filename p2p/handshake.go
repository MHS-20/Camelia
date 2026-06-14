package p2p

import (
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"net"
)

type HandshakeFunc func (net.Conn) (secretKey []byte, iv []byte, peerIV []byte, err error)

func NOPHandshakeFunc(conn net.Conn) (secretKey []byte, iv []byte, peerIV []byte, err error) {
    return nil, nil, nil, nil
}

func DiffieHallmanHandshake(conn net.Conn) (secretKey []byte, iv []byte, peerIV []byte, err error) {
    curve := ecdh.P256()
    privateKey, err := curve.GenerateKey(rand.Reader)
    if err != nil {
        return nil, nil, nil, err
    }
    publicKey := privateKey.PublicKey()
    
    if _, err := conn.Write(publicKey.Bytes()); err != nil {
        return nil, nil, nil, err
    }

    peerPublicKeyBytes := make([]byte, 65) 
    _, err = conn.Read(peerPublicKeyBytes)
    if err != nil {
        return nil, nil, nil, err
    }
    peerPublicKey, err := curve.NewPublicKey(peerPublicKeyBytes)
    if err != nil {
        return nil, nil, nil, err
    }
    
    secretKey, err = privateKey.ECDH(peerPublicKey)
    if err != nil {
        return nil, nil, nil, err
    }

    iv = make([]byte, 16)
    peerIV = make([]byte, 16)
    if _, err := io.ReadFull(rand.Reader, iv); err != nil {
        return nil, nil, nil, err
    }
    if _, err := conn.Write(iv); err != nil {
        return nil, nil, nil, err
    }
    _, err = conn.Read(peerIV)

    return secretKey, iv, peerIV, nil
}
