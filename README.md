# Camelia

Distributed peer-to-peer file storage built on the Kademlia DHT protocol. Files are gzip-compressed, HMAC-authenticated, and AES-CTR encrypted before being content-addressed and replicated across the network without any central server.

<div align="center">
<img src="camelia.png" alt="Logo" width="300"/>
</div>

## Features

- **Decentralised** – No single point of failure. Nodes discover each other via Kademlia distributed hash table routing.
- **End-to-end encryption** – File contents are gzip-compressed, then AES-CTR encrypted with a SHA-256 HMAC integrity signature before leaving the storing node. Wire traffic between peers is additionally encrypted with per-connection ECDH-derived keys.
- **Content-addressed storage** – Files are stored under a SHA-1 path transform (5-character hex directory segments), giving deterministic, collision-resistant addressing.
- **Automatic replication** – When a file is stored, it is broadcast in parallel to all connected peers. Metadata (which node holds which file) is published to the Kademlia DHT.
- **Resilient retrieval** – A file can be retrieved by key from any node in the network. Checks local storage first, verifies HMAC integrity, then queries DHT, then falls back to querying all connected TCP peers in parallel.
- **Partial / range retrieval** – Fetch byte ranges of a stored file without downloading the entire blob.
- **HTTP API** – Optional REST endpoints for `/get`, `/store`, `/stats`, and `/peers`.
- **Peer authentication** – Trust-on-first-use (TOFU) pins each peer's ECDH public key on first connection and rejects key mismatches on reconnection.
- **Rate limiting** – Per-peer token bucket (10 messages/second) drops excess control messages.
- **Persistent peer cache** – On graceful shutdown, saves known peers to `known_peers.json` and reconnects on startup.
- **Connection resilience** – Exponential backoff reconnection with configurable retry limit.
- **Config file support** – JSON config file with env var override precedence.
- **Graceful shutdown** – Handles SIGINT/SIGTERM, saves peer cache, and cleanly shuts down DHT and TCP listener.

## Architecture

Camelia operates in two layers that run inside every node:

| Layer | Protocol | Role |
|---|---|---|
| Kademlia DHT | UDP | Node discovery, routing, key-to-address advertisements |
| File transport | TCP (+ ECDH handshake) | Encrypted file streaming between peers |

**File storage flow**

1. The input data is gzip-compressed.
2. The compressed data is encrypted with AES-CTR using a static key, then a SHA-256 HMAC signature is prepended for integrity verification.
3. The encrypted + signed blob is written to the local content-addressed store (atomic write: temp file + `os.Rename`).
4. A notification is broadcast in parallel to all connected TCP peers, who pull the encrypted data and store it locally.
5. The SHA-256 hash of the human-readable key and the node's own TCP address are published to the Kademlia DHT.

**File retrieval flow**

1. Local storage is checked first. If found, the HMAC signature is verified before decryption.
2. If HMAC verification fails, the local copy is deleted and the system falls through to network retrieval.
3. If absent locally, the Kademlia DHT is queried for the key to find which node holds the file.
4. If the DHT returns a match, the node connects to that peer via TCP and fetches the encrypted data (with HMAC verification).
5. If the DHT lookup fails, the node falls back to querying all directly connected TCP peers in parallel.
6. Once retrieved and verified, the data is decrypted, decompressed, and returned.

**Bootstrapping**

On startup, each node:

1. Loads previously known peers from `known_peers.json` and attempts reconnection.
2. Bootstraps the Kademlia DHT by contacting the configured bootstrap node.
3. Registers its own TCP address in the DHT under its randomly generated Node ID so peers can discover how to connect for file transfers.
4. Connects to any configured TCP bootstrap peers.

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

All tests are run with the Go race detector enabled and include unit tests, integration tests (multi-node store/retrieve, DHT discovery, node failure resilience, concurrent operations), and benchmarks.

### Run benchmarks

```bash
go test -bench=. ./internal/node
go test -bench=. ./p2p
```

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

Each container has a Docker HEALTHCHECK hitting its `/stats` HTTP endpoint. The `test` container waits for the seed to be healthy, then runs the demo test and exits with a success message when the file is verified.

### HTTP API

If `HTTP_ADDR` is set, the node starts an HTTP server with the following endpoints:

| Endpoint | Method | Description |
|---|---|---|
| `/get?key=<key>` | GET | Retrieves a file by key |
| `/store?key=<key>` | POST | Stores a file (body = file data) |
| `/stats` | GET | Returns JSON with `StorageUsedBytes` and `PeerCount` |
| `/peers` | GET | Returns JSON array of connected peer addresses |

The `/stats` endpoint is used by Docker HEALTHCHECK.

### Configuration

Configuration is resolved in order: environment variable > `CONFIG` JSON file > hardcoded default.

| Variable | Default | Description |
|---|---|---|
| `TCP_ADDR` | `:4000` | TCP listen address for file transfers |
| `DHT_ADDR` | `:9000` | UDP listen address for the Kademlia DHT |
| `HTTP_ADDR` | (none) | If set, starts HTTP API on this address |
| `TCP_BOOTSTRAP` | (none) | Comma-separated list of TCP bootstrap peers |
| `DHT_BOOTSTRAP` | (none) | UDP address of the Kademlia bootstrap peer |
| `ENCRYPTION_KEY` | hardcoded fallback | AES encryption key (must be exactly 32 bytes) |
| `STORAGE_ROOT` | `storage/<TCP_ADDR>_network` | Root directory for content-addressed storage |
| `RUN_TEST` | `false` | If `true`, runs the store/retrieve demo test on startup and exits |
| `CONFIG` | `config.json` | Path to JSON config file |

### Config file

The JSON config file supports the same settings as environment variables:

```json
{
  "tcp_addr": ":4000",
  "dht_addr": ":9000",
  "http_addr": ":8080",
  "tcp_bootstrap": "peer1:4000,peer2:4000",
  "dht_bootstrap": "seed:9000",
  "encryption_key": "your-32-byte-aes-key-here",
  "storage_root": "data/store",
  "max_storage_mb": 1024
}
```

`max_storage_mb` sets a storage quota (tracked but not yet enforced with eviction).

## License

GNU General Public License v3.0
