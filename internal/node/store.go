package node

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type PathKey struct {
    Path string
    Filename string
}

func (p *PathKey) GetFilePath() string {
    return fmt.Sprintf("%s/%s", p.Path, p.Filename)
}

func CASPathTransformFunc(key string) PathKey {
    hash := sha1.Sum([]byte(key))
    hashStr := hex.EncodeToString(hash[:])

    blockSize := 5
    sliceLen := len(hashStr) / blockSize

    paths := make([]string, sliceLen)

    for i := 0; i < sliceLen; i++ {
        from, to := i*blockSize, (i*blockSize)+blockSize
        paths[i] = hashStr[from:to]
    }

    return PathKey{ 
        Path: strings.Join(paths, "/"),
        Filename: hashStr,
    }
}

type PathTransformFunc func(string) PathKey

func DefaultPathTransformFunc(key string) PathKey {
    return PathKey{
        Path: key,
        Filename: key,
    }
}

type StoreOpts struct {
    Root string
    PathTransformFunc PathTransformFunc 
}

type Store struct {
    StoreOpts
    mu sync.Mutex
}

func NewStore(opts StoreOpts) *Store {
    if opts.PathTransformFunc == nil {
        opts.PathTransformFunc = DefaultPathTransformFunc
    }
    return &Store{
        StoreOpts: opts,
    }
}

func (s *Store) getAbsolutePath(path string) string {
    return fmt.Sprintf("%s/%s", s.Root, path)
}

func (s *Store) Clear() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    return os.RemoveAll(s.Root)
}

func (s *Store) Has(key string) bool {
    pathKey := s.PathTransformFunc(key)
    _, err := os.Stat(s.getAbsolutePath(pathKey.GetFilePath()))
    return !os.IsNotExist(err)
}

func (s *Store) Delete(key string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    pathKey := s.PathTransformFunc(key)
    defer func(){
        log.Printf("deleted [%s] from disk", s.getAbsolutePath(pathKey.GetFilePath()))
    }()

    err := os.Remove(s.getAbsolutePath(pathKey.GetFilePath()))
    if err != nil {
        return err
    }

    subFolders := strings.Split(s.getAbsolutePath(pathKey.Path), "/")
    for i:=len(subFolders)-1; i >= 0; i-- {
        subPath := strings.Join(subFolders[:i+1], "/")
        dirEntries, err := os.ReadDir(subPath)
        if len(dirEntries) == 0 {
            err = os.Remove(subPath)
        }else{
            break
        }
        if err != nil {
            return err
        }
    }

    return nil
}

func (s *Store) Read(key string) (io.ReadCloser, int64, error) {
	pathKey := s.PathTransformFunc(key)
	filepath := s.getAbsolutePath(pathKey.GetFilePath())
	fi, err := os.Stat(filepath)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(filepath)
	if err != nil {
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

func (s *Store) Write(key string, r io.Reader) (size int64, err error) {
	return s.writeStream(key, r)
}

func (s *Store) writeStream(key string, r io.Reader) (int64, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    pathKey := s.PathTransformFunc(key) 

    if err := os.MkdirAll(s.getAbsolutePath(pathKey.Path), 0700); err != nil {
        return 0, err
    }

    filepath := s.getAbsolutePath(pathKey.GetFilePath())
    tmpFile, err := os.CreateTemp(s.getAbsolutePath(pathKey.Path), "tmp-*")
    if err != nil {
        return 0, err
    }
    tmpFile.Chmod(0600)
    tmpPath := tmpFile.Name()

    n, err := io.Copy(tmpFile, r)
    if err != nil {
        tmpFile.Close()
        os.Remove(tmpPath)
        return 0, err
    }

    if err := tmpFile.Close(); err != nil {
        os.Remove(tmpPath)
        return 0, err
    }

    if err := os.Rename(tmpPath, filepath); err != nil {
        os.Remove(tmpPath)
        return 0, err
    }

    log.Printf("written (%d) bytes to disk: %s", n, filepath)

    return n, nil
}
