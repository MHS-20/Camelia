# Camelia

Distributed peer-to-peer file storage built on the Kademlia DHT protocol. Files are encrypted, content-addressed, and replicated across the network without any central server.

<div align="center">
<img src="camelia.png" alt="Logo" width="300"/>
</div>

## Features

- **Decentralised** – No single point of failure. Nodes discover each other via Kademlia distributed hash table routing.
- **End-to-end encryption** – File contents are AES-CTR encrypted before leaving the storing node. Wire traffic between peers is additionally encrypted with per-connection ECDH-derived keys.
- **Content-addressed storage** – Files are stored under a SHA-1 path transform, giving deterministic, collision-resistant addressing.
- **Automatic replication** – When a file is stored, it is broadcast to all connected peers. Metadata (which node holds which file) is published to the Kademlia DHT.
- **Resilient retrieval** – A file can be retrieved by key from any node in the network. The system checks local storage first, then queries the DHT, then falls back to directly connected peers.

## Architecture

Camelia operates in two layers that run inside every node:

| Layer | Protocol | Role |
|---|---|---|
| Kademlia DHT | UDP | Node discovery, routing, key-to-address advertisements |
| File transport | TCP (+ ECDH handshake) | Encrypted file streaming between peers |

**File storage flow**

1. The file content is encrypted with AES-CTR using a static key.
2. The encrypted data is written to the local content-addressed store.
3. A notification is broadcast to all connected TCP peers, who pull the encrypted data and store it locally.
4. The SHA-256 hash of the human-readable key and the node's own TCP address are published to the Kademlia DHT.

**File retrieval flow**

1. Local storage is checked first.
2. If absent, the Kademlia DHT is queried for the key to find which node holds the file.
3. If the DHT returns a match, the node connects to that peer via TCP and fetches the encrypted data.
4. If the DHT lookup fails, the node falls back to querying all directly connected TCP peers.
5. Once retrieved, the data is decrypted and returned.

**Bootstrapping**

On startup, each node registers its own TCP address in the Kademlia DHT under its randomly generated Node ID, so peers can discover how to connect to it for file transfers.

## Project structure

```
├── main.go              Entry point — configure and run a node or a test
├── server.go            File server — wires DHT, TCP transport, encryption, and storage
├── store.go             Content-addressed disk store (SHA-1 path transform)
├── p2p/                 TCP networking library (transport, handshake, encryption, encoding)
├── Dockerfile           Multi-stage Docker build
├── docker-compose.yml   4-node cluster for integration testing
└── Makefile             Build, run, and test shortcuts
```

## Usage

### Prerequisites

- Go 1.22.1+
- Make (optional)

### Build and run locally

```bash
make build
make run
```

This starts a two-node network, stores a file on the second node, deletes the local copy, and retrieves it from the first node over the P2P network.

### Run tests

```bash
make test
```

All tests are run with the Go race detector enabled.

### Run with Docker Compose

```bash
docker compose up --abort-on-container-exit --exit-code-from test
```

This starts four containers:

| Service | TCP | DHT (UDP) | Role |
|---|---|---|---|
| `seed` | :4000 | :9000 | Bootstrap node |
| `node2` | :4001 | :9001 | Replica |
| `node3` | :4002 | :9002 | Replica |
| `test` | :4003 | :9003 | Stores a file, deletes it locally, then retrieves it from the network |

The `test` container exits with a success message when the file is verified to have been stored and retrieved correctly.

### Configuration

All settings are controlled through environment variables:

| Variable | Default | Description |
|---|---|---|
| `TCP_ADDR` | `:4000` | TCP listen address for file transfers |
| `DHT_ADDR` | `:9000` | UDP listen address for the Kademlia DHT |
| `TCP_BOOTSTRAP` | (empty) | Comma-separated list of TCP bootstrap peers |
| `DHT_BOOTSTRAP` | (empty) | UDP address of the Kademlia bootstrap peer |
| `RUN_TEST` | `false` | Run the store/retrieve demo test on startup |

## License

GNU General Public License v3.0
