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

func NewEncryptionKey() []byte {
    keyBuf := make([]byte, 32)
    io.ReadFull(rand.Reader, keyBuf)
    return keyBuf
}

func CopyDecrypt(key []byte, dst io.Writer, src io.Reader, iv []byte) (int, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return 0, err
    }

    if(iv==nil){
        // Read the id from the given src io.Reader and the size of it will be block.BlockSize()
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

func CopyEncrypt(key []byte, dst io.Writer, src io.Reader, iv []byte) (int, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return 0, err
    }

    totalBytesWrote := 0
    if(iv==nil){
        iv = make([]byte, block.BlockSize())
        if _, err := io.ReadFull(rand.Reader, iv); err != nil {
            return 0, err
        }

        // Prepend the iv to the file so that it can be used for decryption
        if  _, err := dst.Write(iv); err != nil {
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
