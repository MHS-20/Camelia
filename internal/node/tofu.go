package node

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type TofuStore struct {
	mu       sync.Mutex
	filePath string
	peers    map[string]string
}

func NewTofuStore(storageRoot string) (*TofuStore, error) {
	ts := &TofuStore{
		filePath: storageRoot + "/trusted_peers.json",
		peers:    make(map[string]string),
	}
	if err := ts.load(); err != nil {
		return nil, err
	}
	return ts, nil
}

func (ts *TofuStore) load() error {
	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &ts.peers)
}

func (ts *TofuStore) save() error {
	data, err := json.Marshal(ts.peers)
	if err != nil {
		return err
	}
	return os.WriteFile(ts.filePath, data, 0600)
}

func (ts *TofuStore) CheckOrPin(peerID string, publicKey []byte) error {
	hash := sha256.Sum256(publicKey)
	hashStr := hex.EncodeToString(hash[:])

	ts.mu.Lock()
	defer ts.mu.Unlock()

	stored, exists := ts.peers[peerID]
	if !exists {
		ts.peers[peerID] = hashStr
		return ts.save()
	}

	if stored != hashStr {
		return fmt.Errorf("TOFU mismatch for peer %s: expected key hash %s, got %s", peerID, stored, hashStr)
	}

	return nil
}
