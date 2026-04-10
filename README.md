# S3 Filesystem Gateway

Enterprise-grade NFSv4.1-to-S3 gateway written in Go. Exposes S3-compatible object storage as an NFS filesystem.

## Overview

S3 Filesystem Gateway bridges the gap between NFS clients and S3-compatible object storage. Applications mount an S3 bucket over NFS and use standard filesystem operations — the gateway translates them to S3 API calls transparently.

```
NFS Clients → [NFSv4.1 Server] → [S3 Filesystem] → [Cache] → [S3 API] → Object Storage
```

## Features

- **NFSv4.1 protocol** — single port (2049), built-in locking, compound operations, sessions
- **S3-compatible backends** — MinIO, AWS S3, Dell ObjectScale, any S3-compatible storage
- **Two-tier caching** — in-memory LRU + disk-based data cache with ETag coherency
- **POSIX metadata** — uid/gid/mode stored as S3 object metadata headers
- **Write buffering** — local temp file buffering with upload-on-close semantics
- **Observability** — Prometheus metrics, structured logging, health endpoints

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.22+ (for development)

### Run with Docker Compose

```bash
# Start MinIO + gateway
docker compose -f deployments/docker/docker-compose.yml up

# Mount from another terminal (Linux)
mount -t nfs4 localhost:/ /mnt/s3

# Use it
ls /mnt/s3
echo "hello" > /mnt/s3/test.txt
cat /mnt/s3/test.txt
```

### Build from Source

```bash
make build
./bin/s3nfsgw --config configs/default.yaml
```

## Configuration

Configuration via YAML file with environment variable overrides:

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `S3_ENDPOINT` | `localhost:9000` | S3 endpoint |
| `S3_ACCESS_KEY` | `minioadmin` | Access key |
| `S3_SECRET_KEY` | `minioadmin` | Secret key |
| `S3_BUCKET` | `data` | Bucket name |
| `S3_USE_SSL` | `false` | Enable TLS |
| `NFS_PORT` | `2049` | NFS listen port |
| `CACHE_METADATA_TTL` | `60s` | Metadata cache TTL |
| `CACHE_DATA_DIR` | `/var/cache/s3gw` | Data cache directory |
| `CACHE_DATA_MAX_SIZE` | `10GB` | Data cache max size |
| `LOG_LEVEL` | `info` | Log level |

## Architecture

### Layers

- **NFS Layer** (`smallfz/libnfs-go`) — Pure Go NFSv4 server, handles protocol, sessions, locking
- **S3 Filesystem** — Implements libnfs-go `fs.FS` interface, translates POSIX ops to S3
- **Cache Layer** — In-memory metadata LRU + bbolt persistence + disk-based data cache
- **S3 Backend** (`minio/minio-go/v7`) — S3 client with connection pooling and multipart uploads

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| NFSv4.1 over NFSv3 | Single port, built-in locking, sessions, compound ops |
| libnfs-go over go-nfs | Pure Go NFSv4 with clean fs.FS interface, zero deps |
| minio-go over aws-sdk-go | First-class MinIO support, lighter weight |
| bbolt for metadata | ACID, embedded, zero-config, read-optimized |
| Upload on close | Optimized for write-once/read-many workloads |

### S3 Limitations (by design)

- **No atomic rename** — implemented as CopyObject + DeleteObject
- **No symlinks/hardlinks** — S3 has no equivalent
- **Random writes are expensive** — requires download-modify-upload cycle
- **Eventual consistency** — configurable cache TTL trades freshness for performance

## S3 Compatibility

| Backend | Status |
|---------|--------|
| MinIO | Primary test target |
| AWS S3 | Supported |
| Dell ObjectScale | Planned |

## Development

```bash
make build          # Build binary
make test           # Run unit tests
make lint           # Run linter
make docker         # Build Docker image
make up             # Start docker-compose environment
make down           # Stop docker-compose environment
make integration    # Run integration tests
```

## License

Apache License 2.0
