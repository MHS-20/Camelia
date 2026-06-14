package p2p

import (
	"crypto/ecdh"
	"crypto/rand"
	"io"
	"net"
)

// HandshakeFunc performs an initial handshake on a connection and returns derived key material.
type HandshakeFunc func(net.Conn) (secretKey []byte, iv []byte, peerIV []byte, peerPublicKey []byte, err error)

// NOPHandshakeFunc is a no-op handshake that returns nil for all values.
func NOPHandshakeFunc(conn net.Conn) (secretKey []byte, iv []byte, peerIV []byte, peerPublicKey []byte, err error) {
	return nil, nil, nil, nil, nil
}

// DiffieHellmanHandshake performs an ECDH P256 key exchange and exchanges random IVs.
func DiffieHellmanHandshake(conn net.Conn) (secretKey []byte, iv []byte, peerIV []byte, peerPublicKey []byte, err error) {
	curve := ecdh.P256()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	publicKey := privateKey.PublicKey()

	if _, err := conn.Write(publicKey.Bytes()); err != nil {
		return nil, nil, nil, nil, err
	}

	peerPublicKeyBytes := make([]byte, 65)
	_, err = conn.Read(peerPublicKeyBytes)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	peerPublicKeyParsed, err := curve.NewPublicKey(peerPublicKeyBytes)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	secretKey, err = privateKey.ECDH(peerPublicKeyParsed)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	iv = make([]byte, 16)
	peerIV = make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, nil, nil, nil, err
	}
	if _, err := conn.Write(iv); err != nil {
		return nil, nil, nil, nil, err
	}
	_, err = conn.Read(peerIV)

	return secretKey, iv, peerIV, peerPublicKeyBytes, nil
}
