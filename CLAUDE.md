# S3 Filesystem Gateway

## Project Overview

Enterprise-grade NFSv4.1-to-S3 gateway written in Go. Exposes S3-compatible object storage (MinIO, AWS S3, Dell ObjectScale) as an NFSv4 filesystem.

## Architecture

```
                           +-----------------+
                           |   NFS Clients   |
                           +--------+--------+
                                    |
                                    v
                      +-------------+-------------+
                      |  NFSv4.1 Server (libnfs-go)|
                      |  Port 2049                 |
                      |  Per-session S3FS instances |
                      +-------------+--------------+
                                    |
                      +-------------+--------------+
                      |  S3 Filesystem (fs.FS)      |
                      |  POSIX ops -> S3 API calls  |
                      |  Handle/inode management    |
                      +---+------------------+------+
                          |                  |
               +----------+------+   +-------+--------+
               | Metadata Cache  |   | Data Cache     |
               | In-memory LRU   |   | Disk-based LRU |
               | TTL expiry      |   | ETag coherency |
               | Negative cache  |   | Shard storage  |
               +----------+------+   +-------+--------+
                          |                  |
                      +---+------------------+------+
                      |  S3 Client (minio-go)       |
                      |  Connection pooling          |
                      |  Ranged reads, multipart     |
                      +-------------+---------------+
                                    |
                           +--------+--------+
                           |  Object Storage |
                           |  (MinIO/S3/etc) |
                           +-----------------+

          +-------------------+
          | Health Server     |  Port 9090 (default)
          | /health /ready    |
          | /metrics (Prom)   |
          +-------------------+
```

## Key Dependencies

| Package | Version | Purpose |
|---|---|---|
| `github.com/smallfz/libnfs-go` | v0.0.7 | NFSv4 server (session mgmt, compound ops, built-in locking) |
| `github.com/minio/minio-go/v7` | v7.0.100 | S3 client (MinIO-native, ObjectScale compatible) |
| `go.etcd.io/bbolt` | v1.4.3 | Embedded metadata DB (inode mapping, handle persistence) |
| `github.com/prometheus/client_golang` | v1.23.2 | Prometheus metrics (counters, histograms, gauges) |

No viper/cobra -- config uses stdlib `os.Getenv` with hardcoded defaults.

## Build and Run

```bash
make build          # Build binary to bin/s3nfsgw
make test           # Run unit tests
make lint           # Run golangci-lint
make docker         # Build Docker image
make up             # Start gateway + MinIO via docker-compose
make down           # Stop docker-compose environment
make all            # fmt + vet + lint + test + build
```

## Test Environment

- MinIO container on ports 9000 (API) / 9001 (console)
- Gateway on port 2049 (NFS), port 9090 (health/metrics)
- Default creds: minioadmin / minioadmin
- Mount: `sudo mount -t nfs4 localhost:/ /mnt/s3`

## Key Files

### Entry Point
- `cmd/s3nfsgw/main.go` -- CLI entry point, flag parsing, startup sequence (S3 client -> health server -> handle store -> NFS server), graceful shutdown on SIGINT/SIGTERM

### NFS Layer
- `internal/nfs/server.go` -- Wraps libnfs-go server, creates per-session S3FS via `vfsLoader` closure, passes shared metadata cache

### S3 Filesystem (`internal/s3fs/`)
- `filesystem.go` -- `S3FS` struct implementing `libnfs-go/fs.FS` (~15 methods: Open, OpenFile, Stat, Rename, Remove, MkdirAll, etc.)
- `file.go` -- `s3File` (read-only, uses chunkReader), `s3WritableFile` (temp file buffer, upload on Close), `dirFile` (minimal dir stub)
- `reader.go` -- `chunkReader` with adaptive prefetch (1MB -> 4MB -> 16MB based on sequential access detection)
- `handle.go` -- `HandleStore` using bbolt for persistent bidirectional inode <-> S3 key mapping, in-memory cache for fast lookups
- `attrs.go` -- `fileInfo` implementing `nfs.FileInfo`, POSIX metadata parsing from S3 user-metadata headers
- `cache_integration.go` -- Helper methods on S3FS for cache get/put/invalidate, directory listing cache, parent invalidation

### Cache Layer (`internal/cache/`)
- `metadata.go` -- `MetadataCache`: in-memory LRU (10k entries), TTL-based expiry (files 300s, dirs 60s, negative 10s), background eviction, directory listing cache
- `data.go` -- `DataCache`: disk-based LRU with SHA256-keyed shard dirs, atomic writes (temp+rename), startup scan to rebuild index, configurable max size (default 10GB)

### S3 Layer
- `internal/s3/client.go` -- `Client` wrapping minio-go with connection pooling (100 idle conns). Methods: HeadObject, GetObject, GetObjectRange, ListObjects, PutObject, DeleteObject, CopyObject, CreateDirMarker, BucketExists

### Observability
- `internal/metrics/prometheus.go` -- Metric definitions (NFS ops, S3 requests, cache hits/misses, active connections, bytes transferred) with recording helper functions
- `internal/health/health.go` -- HTTP server with `/health` (liveness), `/ready` (S3 bucket reachability), `/metrics` (Prometheus handler)

### Config
- `internal/config/config.go` -- Config structs (S3, NFS, Health, Cache, Log), defaults, env var overrides. No YAML loading yet (TODO).

### Deployment
- `deployments/docker/Dockerfile` -- Multi-stage build (golang:1.22-alpine -> alpine:3.19)
- `deployments/docker/docker-compose.yml` -- MinIO + gateway + minio-init (creates bucket)

## Key Design Decisions

- **NFSv4.1 via libnfs-go:** Single port 2049, built-in locking, compound ops, sessions. Clean fs.FS interface. Reference `memfs` package for implementation patterns.
- **minio-go over aws-sdk-go:** First-class MinIO/ObjectScale support, lighter weight.
- **bbolt for handles:** ACID, embedded, zero-config, read-optimized B+tree. In-memory cache avoids disk reads on hot path.
- **Write strategy:** Buffer to local temp file, upload on close (write-once/read-many optimized). POSIX metadata written as S3 user-metadata.
- **Directory handling:** Marker objects (trailing `/`) + implicit dirs from ListObjectsV2 prefixes.
- **Inode management:** Synthetic monotonic counter in bbolt, 8-byte big-endian handle encoding (memfs pattern).
- **Adaptive prefetch:** Starts at 1MB chunks, grows to 4MB after 4 sequential reads, 16MB after 12. Resets on seek.
- **Cache invalidation:** Writes invalidate the affected key + parent directory listing to ensure consistency.
- **Negative caching:** Non-existent paths cached for 10s to reduce S3 HEAD traffic on stat storms.

## Conventions

- Go module: `github.com/vipurkumar/s3-filesystem-gateway`
- Config: Hardcoded defaults with environment variable overrides (`S3_ENDPOINT`, `S3_ACCESS_KEY`, `NFS_PORT`, `HEALTH_PORT`, etc.)
- Logging: `log/slog` structured JSON to stdout
- Metrics: Prometheus on `:9090` (configurable via `HEALTH_PORT`)
- Code layout: `internal/` (private packages), `cmd/s3nfsgw/` (CLI entry point)
- S3 filesystem implements `libnfs-go/fs.FS` interface (not go-billy)
- Error handling: `fmt.Errorf` with `%w` wrapping, `os.ErrNotExist` for not-found
- Concurrency: `sync.Mutex` / `sync.RWMutex` per struct, no global locks
- S3 keys: root `/` maps to empty string `""`, paths stripped of leading `/`, dirs end with `/`

## S3 Compatibility Targets

1. **MinIO** -- Primary test target (docker-compose included)
2. **AWS S3** -- Standard compatibility
3. **Dell ObjectScale** -- Ports 9020/9021, path-style addressing, S3 API subset

## NFSv4 Library References

- **libnfs-go memfs:** Reference implementation for fs.FS interface (`github.com/smallfz/libnfs-go/memfs/`)
- **buildbarn NFSv4:** Reference for protocol edge cases (`github.com/buildbarn/bb-remote-execution/pkg/filesystem/virtual/nfsv4/`)
