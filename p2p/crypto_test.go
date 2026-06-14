package p2p

import (
	"bytes"
	"testing"
)

func TestCopyEncryptDecrypt(t *testing.T) {
    data := "private data code 101"
    src := bytes.NewReader([]byte(data))
    cipherOut := new(bytes.Buffer)
    key := []byte("asdwedscfrgtvfdybhghnjhgrfdrefrd")

    _, err := CopyEncrypt(key, cipherOut, src, nil)
    if err != nil {
        t.Error(err)
    }

    if 16+len(data) != cipherOut.Len() {
        t.Fail()
    }

    decryptOut := new(bytes.Buffer)
    _, err = CopyDecrypt(key, decryptOut, cipherOut, nil)
    if err != nil {
        t.Error(err)
    }

    if decryptOut.String() != data {
        t.Errorf("decryption failed")
    }
}

func TestCopyEncryptDecryptHMAC(t *testing.T) {
    data := "private data with integrity"
    src := bytes.NewReader([]byte(data))
    cipherOut := new(bytes.Buffer)
    key := []byte("asdwedscfrgtvfdybhghnjhgrfdrefrd")

    _, err := CopyEncryptHMAC(key, cipherOut, src)
    if err != nil {
        t.Error(err)
    }

    decryptOut := new(bytes.Buffer)
    _, err = CopyDecryptHMAC(key, decryptOut, cipherOut)
    if err != nil {
        t.Error(err)
    }

    if decryptOut.String() != data {
        t.Errorf("decryption failed: got %q, want %q", decryptOut.String(), data)
    }
}

func TestCopyDecryptHMAC_Tampered(t *testing.T) {
    data := "tamper test data"
    src := bytes.NewReader([]byte(data))
    cipherOut := new(bytes.Buffer)
    key := []byte("asdwedscfrgtvfdybhghnjhgrfdrefrd")

    _, err := CopyEncryptHMAC(key, cipherOut, src)
    if err != nil {
        t.Error(err)
    }

    tampered := cipherOut.Bytes()
    tampered[len(tampered)-1] ^= 0xFF

    decryptOut := new(bytes.Buffer)
    _, err = CopyDecryptHMAC(key, decryptOut, bytes.NewReader(tampered))
    if err == nil {
        t.Error("expected HMAC error for tampered data, got nil")
    }
}
