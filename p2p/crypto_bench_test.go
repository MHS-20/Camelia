package p2p

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
)

func BenchmarkCopyEncrypt(b *testing.B) {
	key := make([]byte, 32)
	rand.Read(key)

	sizes := []int{64, 4096, 1048576}
	for _, size := range sizes {
		data := make([]byte, size)
		rand.Read(data)

		b.Run(fmt.Sprintf("encrypt_size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			for i := 0; i < b.N; i++ {
				src := bytes.NewReader(data)
				dst := bytes.NewBuffer(nil)
				if _, err := CopyEncrypt(key, dst, src, nil); err != nil {
					b.Fatal(err)
				}
			}
		})

		encrypted := bytes.NewBuffer(nil)
		CopyEncrypt(key, encrypted, bytes.NewReader(data), nil)

		b.Run(fmt.Sprintf("decrypt_size_%d", size), func(b *testing.B) {
			b.SetBytes(int64(size))
			for i := 0; i < b.N; i++ {
				dst := bytes.NewBuffer(nil)
				if _, err := CopyDecrypt(key, dst, bytes.NewReader(encrypted.Bytes()), nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
