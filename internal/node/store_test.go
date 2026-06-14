package node

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func TestPathTransformFunc(t *testing.T) {
    key := "yo its a dummy string"
    pathKey := CASPathTransformFunc(key)
    filename := "90555eb6014736bedad917345c0193cd1f638ad6"
    expectedPathName := "90555/eb601/4736b/edad9/17345/c0193/cd1f6/38ad6"
    if pathKey.Filename != filename { 
        t.Errorf("have filename %s want %s", pathKey.Filename, filename)
    }
    if pathKey.Path != expectedPathName { 
        t.Errorf("have path name %s want %s", pathKey.Path, expectedPathName)
    }
}

func TestStore(t *testing.T) {
    store := newStoreForTest(t)

    for i:=0; i < 50; i++ {
        key := fmt.Sprintf("foo_%d", i)
        data := []byte("some random bytes")
        if _, err := store.writeStream(key, bytes.NewReader(data)); err != nil {
            t.Error(err)
        }

        if !store.Has(key) {
            t.Errorf("expected to have key %s", key)
        }

		r, _, err := store.Read(key)
		if err != nil {
			t.Error(err)
		}

		b, err := io.ReadAll(r)
		if err != nil {
			t.Error(err)
		}
		r.Close()

        if string(b) != string(data) {
            t.Errorf("want \"%s\" have \"%s\"", data, b)
        }

        if err := store.Delete(key); err != nil {
            t.Error(err)
        }

        if store.Has(key) {
            t.Errorf("not expecting the key %s", key)
        }
    }
}

func newStoreForTest(t testing.TB) *Store {
    t.Helper()
    opts := StoreOpts{
        Root: t.TempDir(),
        PathTransformFunc: CASPathTransformFunc,
    }
    return NewStore(opts)
}

func TestStoreWritePublic(t *testing.T) {
	store := newStoreForTest(t)

	data := []byte("test public write method")
	key := "public_write_test"

	n, err := store.Write(key, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(data)) {
		t.Fatalf("expected %d, got %d", len(data), n)
	}

	if !store.Has(key) {
		t.Fatal("expected key to exist")
	}

	r, _, err := store.Read(key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	if string(got) != string(data) {
		t.Fatalf("expected %q, got %q", data, got)
	}
}

func TestStoreReadRange(t *testing.T) {
	store := newStoreForTest(t)

	data := []byte("hello world range test")
	key := "range_test_key"
	_, err := store.Write(key, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	r, n, err := store.ReadRange(key, 6, 5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	if string(got) != "world" {
		t.Fatalf("expected 'world', got %q", got)
	}
}
