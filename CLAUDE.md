# S3 Filesystem Gateway

## Project Overview

Enterprise-grade NFSv4.1-to-S3 gateway written in Go. Exposes S3-compatible object storage (MinIO, AWS S3, Dell ObjectScale) as an NFSv4 filesystem.

## Architecture

```
NFS Clients → [NFSv4.1 Server (libnfs-go)] → [S3 Filesystem (fs.FS)] → [Cache] → [S3 (minio-go)] → Object Storage
```

- **NFS Layer:** `smallfz/libnfs-go` — Pure Go NFSv4 server, zero external deps, MIT license
- **S3 Filesystem:** Implements libnfs-go `fs.FS` interface (~15 methods) backed by S3
- **Cache Layer:** In-memory LRU + `go.etcd.io/bbolt` (metadata) + disk LRU (data)
- **S3 Layer:** `minio/minio-go/v7` — S3 operations, multipart uploads

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/smallfz/libnfs-go` | NFSv4 server (session mgmt, compound ops, built-in locking) |
| `github.com/minio/minio-go/v7` | S3 client (MinIO-native, ObjectScale compatible) |
| `go.etcd.io/bbolt` | Embedded metadata DB (inode mapping, attrs cache) |
| `github.com/spf13/viper` | Config (YAML + env vars) |
| `github.com/prometheus/client_golang` | Metrics |

## Build & Run

```bash
make build          # Build binary to bin/s3nfsgw
make test           # Run unit tests
make lint           # Run golangci-lint
make docker         # Build Docker image
docker compose up   # Start gateway + MinIO
```

## Test Environment

- MinIO container on ports 9000 (API) / 9001 (console)
- Gateway on port 2049 (NFS)
- Default creds: minioadmin / minioadmin
- Mount: `mount -t nfs4 localhost:/ /mnt/s3`

## Key Design Decisions

- **NFSv4.1 via libnfs-go over NFSv3 via go-nfs:** Single port 2049, built-in locking, compound ops, sessions. libnfs-go has clean fs.FS interface with zero deps. Reference `memfs` package for implementation patterns.
- **minio-go over aws-sdk-go:** First-class MinIO/ObjectScale support, lighter weight
- **bbolt for metadata:** ACID, embedded, zero-config, read-optimized B+tree
- **Write strategy:** Buffer to local temp file, upload on close (write-once/read-many optimized)
- **Directory handling:** Marker objects (trailing `/`) + implicit dirs from ListObjectsV2 prefixes
- **Inode/handle management:** Synthetic monotonic counter in bbolt, 8-byte handle encoding (memfs pattern)
- **POSIX metadata:** Stored as S3 user-metadata headers (x-amz-meta-uid/gid/mode), configurable defaults

## Conventions

- Go module: `github.com/rupivbluegreen/s3-filesystem-gateway`
- Config: YAML file with environment variable overrides (S3_ENDPOINT, S3_ACCESS_KEY, etc.)
- Logging: `log/slog` structured JSON
- Metrics: Prometheus on :9090
- Code in `internal/` (private), CLI in `cmd/s3nfsgw/`
- S3 filesystem implements `libnfs-go/fs.FS` interface (not go-billy)

## S3 Compatibility Targets

1. **MinIO** — Primary test target
2. **AWS S3** — Standard compatibility
3. **Dell ObjectScale** — Ports 9020/9021, path-style addressing, S3 API subset

## NFSv4 Library References

- **libnfs-go memfs:** Reference implementation for fs.FS interface (`github.com/smallfz/libnfs-go/memfs/`)
- **buildbarn NFSv4:** Reference for protocol edge cases (`github.com/buildbarn/bb-remote-execution/pkg/filesystem/virtual/nfsv4/`)
