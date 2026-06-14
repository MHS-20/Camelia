package node

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"testing"
)

func BenchmarkStoreWrite(b *testing.B) {
	store := newStoreForTest(b)

	sizes := []int{64, 4096, 1048576}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			data := make([]byte, size)
			rand.Read(data)
			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("bench_write_%d", i)
				if _, err := store.Write(key, bytes.NewReader(data)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStoreRead(b *testing.B) {
	store := newStoreForTest(b)

	sizes := []int{64, 4096, 1048576}
	for _, size := range sizes {
		key := fmt.Sprintf("bench_read_%d", size)
		data := make([]byte, size)
		rand.Read(data)
		if _, err := store.Write(key, bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}

		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				r, _, err := store.Read(key)
				if err != nil {
					b.Fatal(err)
				}
				io.Copy(io.Discard, r)
				r.Close()
			}
		})
	}
}
