package p2p

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"
)

const copyBufSize = 32 * 1024

var copyBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, copyBufSize)
		return &buf
	},
}

// NewEncryptionKey generates a random 32-byte AES key.
func NewEncryptionKey() []byte {
	keyBuf := make([]byte, 32)
	io.ReadFull(rand.Reader, keyBuf)
	return keyBuf
}

// CopyDecrypt decrypts AES-CTR data from src into dst. When iv is nil the IV is read from src.
func CopyDecrypt(key []byte, dst io.Writer, src io.Reader, iv []byte) (int, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}

	if iv == nil {
		iv = make([]byte, block.BlockSize())
		if _, err := src.Read(iv); err != nil {
			return 0, err
		}
	}

	totalBytesWrote := 0

	bufPtr := copyBufPool.Get().(*[]byte)
	buf := *bufPtr
	defer copyBufPool.Put(bufPtr)

	stream := cipher.NewCTR(block, iv)

	for {
		var wn int
		rn, err := src.Read(buf)
		if rn > 0 {
			stream.XORKeyStream(buf, buf[:rn])
			if wn, err = dst.Write(buf[:rn]); err != nil {
				return 0, err
			}
			totalBytesWrote += wn
		}

		if err == io.EOF {
			break
		}
	}

	return totalBytesWrote, nil
}

// CopyEncrypt encrypts data from src with AES-CTR into dst. When iv is nil a random IV is prepended.
func CopyEncrypt(key []byte, dst io.Writer, src io.Reader, iv []byte) (int, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}

	totalBytesWrote := 0
	if iv == nil {
		iv = make([]byte, block.BlockSize())
		if _, err := io.ReadFull(rand.Reader, iv); err != nil {
			return 0, err
		}
		if _, err := dst.Write(iv); err != nil {
			return 0, err
		}
		totalBytesWrote = len(iv)
	}

	bufPtr := copyBufPool.Get().(*[]byte)
	buf := *bufPtr
	defer copyBufPool.Put(bufPtr)

	stream := cipher.NewCTR(block, iv)

	for {
		var wn int
		rn, err := src.Read(buf)
		if rn > 0 {
			stream.XORKeyStream(buf, buf[:rn])
			if wn, err = dst.Write(buf[:rn]); err != nil {
				return 0, err
			}
			totalBytesWrote += wn
		}

		if err == io.EOF {
			break
		}
	}

	return totalBytesWrote, nil
}

// CopyEncryptHMAC encrypts data with AES-CTR and prepends a SHA-256 HMAC signature for integrity.
func CopyEncryptHMAC(key []byte, dst io.Writer, src io.Reader) (int, error) {
	encBuf := new(bytes.Buffer)
	n, err := CopyEncrypt(key, encBuf, src, nil)
	if err != nil {
		return 0, err
	}
	encrypted := encBuf.Bytes()

	mac := hmac.New(sha256.New, key)
	mac.Write(encrypted)
	signature := mac.Sum(nil)

	if _, err := dst.Write(signature); err != nil {
		return 0, err
	}
	if _, err := dst.Write(encrypted); err != nil {
		return 0, err
	}
	return n + sha256.Size, nil
}

// CopyDecryptHMAC reads a SHA-256 HMAC signature, verifies it, then decrypts the data with AES-CTR.
func CopyDecryptHMAC(key []byte, dst io.Writer, src io.Reader) (int, error) {
	signature := make([]byte, sha256.Size)
	if _, err := io.ReadFull(src, signature); err != nil {
		return 0, fmt.Errorf("failed to read HMAC: %w", err)
	}

	encrypted, err := io.ReadAll(src)
	if err != nil {
		return 0, fmt.Errorf("failed to read encrypted data: %w", err)
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(encrypted)
	expected := mac.Sum(nil)

	if !hmac.Equal(signature, expected) {
		return 0, fmt.Errorf("HMAC mismatch: data integrity check failed")
	}

	return CopyDecrypt(key, dst, bytes.NewReader(encrypted), nil)
}
