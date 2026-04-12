# Feature Parity: S3 Filesystem Gateway vs AWS S3 Files

**Last updated:** 2026-04-13

This document serves as both a competitive comparison and an implementation roadmap. It maps our gateway's capabilities against [AWS S3 Files](https://aws.amazon.com/s3/features/files/), identifies gaps, and prioritizes work to close them.

**AWS Sources:**
- [S3 Files Features](https://aws.amazon.com/s3/features/files/)
- [Launch Blog Post](https://aws.amazon.com/blogs/aws/launching-s3-files-making-s3-buckets-accessible-as-file-systems/)
- [S3 Files Documentation](https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-files.html)
- [Unsupported Features, Limits, Quotas](https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-files-quotas.html)

## Status Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Implemented |
| 🟡 | Partial — code exists but incomplete or not wired in |
| 📋 | Planned — on roadmap, design understood |
| ⬜ | Gap — AWS has it, we lack it, not yet planned |
| N/A | Does not apply to this deployment model |

## Deployment Model Differences

| Aspect | AWS S3 Files | S3 Filesystem Gateway |
|--------|-------------|----------------------|
| Deployment | Managed service (EFS-backed) | Self-hosted single binary |
| S3 Backends | AWS S3 only | MinIO, AWS S3, Dell ObjectScale |
| Protocol | NFSv4.1 / NFSv4.2 | NFSv4.1 |
| Pricing | Pay-per-use ($0.30/GB-mo cache, $0.03/GB read, $0.06/GB write) | Free, open source (Apache 2.0) |
| Compute Targets | EC2, EKS, ECS, Lambda | Any NFS client (Linux, macOS, Windows) |
| Scaling | Auto-scales throughput/IOPS | Operator-managed (single instance) |

---

## Feature Comparison

### Protocol & Access

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| NFSv4.1 support | Yes | Yes (libnfs-go) | ✅ | — | |
| NFSv4.2 support | Yes | No | ⬜ | P2 | Depends on libnfs-go upstream |
| Standard NFS mount | Yes | Yes (`mount -t nfs4`) | ✅ | — | |
| POSIX file operations (open, read, write, close, stat) | Yes | Yes | ✅ | — | |
| Directory operations (mkdir, readdir, rmdir) | Yes | Yes | ✅ | — | |
| Rename | Yes | Yes (copy + delete) | ✅ | — | Not atomic (S3 limitation, same as AWS) |
| Seek | Yes | Yes | ✅ | — | |
| Symlinks | Yes (target ≤4,080 bytes) | No (returns error) | ✅ | P2 | Via S3 marker objects with metadata |
| Hard links | No | No | ✅ | — | Neither supports this |
| File locking (advisory) | Yes (512 locks/file) | Yes (via libnfs-go) | ✅ | — | |
| Mandatory locking | No | No | ✅ | — | Neither supports this |
| Dual access (file + object) | Yes (simultaneous) | Partial (NFS-only; S3 direct still works) | 🟡 | P1 | Need consistency guarantees between NFS and direct S3 access |
| Read-after-write consistency | Yes | No (eventual + cache TTL) | ✅ | P1 | Cache refresh on write close |
| Close-to-open consistency | Yes | No | 🟡 | P1 | Subject to metadata cache TTL |
| POSIX permissions (uid/gid/mode) | Yes (stored as S3 metadata) | Yes (`x-amz-meta-uid/gid/mode`) | ✅ | — | Same approach |
| chmod / chown | Yes | No (returns ENOTSUP) | ✅ | P2 | Via CopyObject with metadata replace |
| Truncate | Yes | Partial (returns error) | ✅ | P2 | Via empty object upload |
| Max file name length | 255 bytes | 1,024 bytes (MaxName) | ✅ | — | We allow longer names |
| Max S3 key length | 1,024 bytes | 1,024 bytes (S3 limit) | ✅ | — | |
| Max directory depth | 1,000 levels | Unlimited | ✅ | — | |

### Read Performance

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| Adaptive prefetch | Implicit (EFS + S3 routing) | Yes (1MB → 4MB → 16MB chunks) | ✅ | — | `internal/s3fs/reader.go` |
| Data cache for reads | Yes (EFS high-perf storage) | Code exists, not wired into read path | ✅ | **P0** | Disk-based LRU with ETag coherency |
| Intelligent read routing | Yes (small <128KB from cache, large sequential from S3) | No | ✅ | P1 | Small files (≤128KB) cached, large streamed from S3 |
| ETag-based cache coherency | Implicit (sync handles it) | Yes (in data cache) | ✅ | — | `internal/cache/data.go` |
| Range read support | Yes | Yes (`GetObjectRange`) | ✅ | — | |
| Aggregate throughput | Up to TB/s (managed fleet) | Bounded by operator hardware | N/A | — | Infrastructure, not software |
| Sub-millisecond latency (cached) | Yes (~1ms for active data) | Depends on disk cache (not yet active) | ✅ | P0 | Achievable once data cache wired in |
| Max read size per op | Not documented | 1 MB (MaxRead) | ✅ | — | |

### Write Performance

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| Write buffering | Yes (EFS + async sync to S3) | Yes (local temp file) | ✅ | — | Upload on `Close()` |
| Write-on-close semantics | Yes | Yes | ✅ | — | |
| Multipart upload for large files | Yes (implicit) | No (single PutObject) | 📋 | P2 | Add for files > 5GB |
| Random write optimization | Yes (EFS handles it) | No (download-modify-upload) | ⬜ | P3 | S3 limitation; AWS masks it with EFS |
| Max write size per op | Not documented | 1 MB (MaxWrite) | ✅ | — | |
| Max file size | 48 TiB | Limited by temp disk space | 🟡 | P2 | Need multipart upload for large files |

### Caching & Data Management

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| Metadata cache (in-memory) | Yes (EFS) | Yes (LRU, 10K entries) | ✅ | — | `internal/cache/metadata.go` |
| File TTL | Managed | 300s (configurable) | ✅ | — | |
| Directory TTL | Managed | 60s (configurable) | ✅ | — | |
| Negative caching | Not documented | Yes (10s TTL) | ✅ | — | Reduces S3 HEAD traffic |
| Disk-based data cache | Yes (EFS high-perf storage) | Yes (SHA256-keyed, LRU) | ✅ | **P0** | |
| Configurable cache expiration | Yes (1–365 days, default 30) | No (fixed TTLs) | ✅ | P1 | Via CACHE_METADATA_TTL and data cache TTL |
| Configurable file size threshold | Yes (default 128KB) | No | ✅ | P1 | 128KB threshold for cache routing |
| Bidirectional sync | Yes (auto import/export) | No (read-through cache only) | ⬜ | P3 | Complex; lower priority for self-hosted |
| Working set auto-loading | Yes (lazy on first access) | No | ⬜ | P3 | Pre-warming/lazy loading pattern |
| Cache size limit | Managed (pay for usage) | Configurable (default 10GB) | ✅ | — | `CACHE_DATA_MAX_SIZE` env var |
| Background eviction | Managed | Yes (30s interval) | ✅ | — | Both metadata and data caches |

### Security

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| TLS to storage backend | Yes (TLS 1.3) | Yes (configurable via `S3_USE_SSL`) | ✅ | — | |
| NFS traffic encryption | Yes (TLS in transit) | No (plaintext NFS) | ⬜ | P2 | Requires stunnel or NFS-over-TLS |
| Kerberos authentication | No | No | ✅ | — | Neither supports this |
| POSIX permissions enforcement | Yes | Yes (uid/gid/mode in S3 metadata) | ✅ | — | |
| Encryption at rest | Yes (SSE-S3 or SSE-KMS) | Depends on backend | N/A | — | S3 backend responsibility |
| Non-root container | N/A (managed) | Yes (uid 10001) | ✅ | — | |
| Path traversal protection | N/A (managed) | Yes (path validation) | ✅ | — | |
| Credential protection | IAM roles | Env vars (never logged) | ✅ | — | |
| IAM integration | Yes (native) | No | N/A | — | Not applicable to self-hosted |
| NFS access points | Yes (app-specific entry points) | No | ⬜ | P3 | Per-app mount scoping |
| HTTP server hardening | N/A | Yes (timeouts, max headers) | ✅ | — | Health server hardened |
| Rate limiting | Yes (managed) | No | ✅ | P2 | Token bucket limiter in internal/ratelimit |

### Scalability

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| Connection pooling (to S3) | Managed | Yes (100 idle conns) | ✅ | — | |
| Concurrent NFS connections | 25,000 per file system | Bounded by OS/hardware | N/A | — | Infrastructure limit |
| Auto-scaling throughput/IOPS | Yes | No (single instance) | N/A | — | Managed service feature |
| Horizontal scaling | N/A (managed) | No (single instance) | 📋 | P3 | Multi-instance with shared cache coherency |
| Max open files per client | 32,768 | Bounded by OS | N/A | — | |
| Mount targets per AZ | 1 | Unlimited | ✅ | — | |

### Observability

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| NFS operation metrics | CloudWatch | Prometheus (`s3gw_nfs_operations_total`) | ✅ | — | |
| NFS operation latency | CloudWatch | Prometheus histogram (`s3gw_nfs_operation_duration_seconds`) | ✅ | — | |
| S3 request metrics | CloudWatch | Prometheus (`s3gw_s3_requests_total`) | ✅ | — | |
| Cache hit/miss rates | Not exposed | Prometheus (`s3gw_cache_hits_total` / `misses`) | ✅ | — | |
| Active connections gauge | CloudWatch | Prometheus (`s3gw_active_connections`) | ✅ | — | |
| Bytes transferred | CloudWatch | Prometheus (`s3gw_bytes_transferred_total`) | ✅ | — | |
| Health endpoint (liveness) | N/A (managed) | `/health` (always 200) | ✅ | — | |
| Readiness endpoint | N/A (managed) | `/ready` (checks S3 bucket) | ✅ | — | |
| Metrics scrape endpoint | CloudWatch only | `/metrics` (Prometheus) | ✅ | — | |
| Pre-built dashboards | CloudWatch dashboards | No | 📋 | P3 | Grafana dashboard templates |

### Operations & Deployment

| Feature | AWS S3 Files | Our Gateway | Status | Priority | Notes |
|---------|-------------|-------------|--------|----------|-------|
| Docker deployment | N/A (managed) | Yes (multi-stage Dockerfile) | ✅ | — | |
| Docker Compose (dev) | N/A | Yes (MinIO + gateway) | ✅ | — | |
| Graceful shutdown | Managed | Yes (SIGINT/SIGTERM, 30s timeout) | ✅ | — | |
| YAML config file | N/A (console/API) | Partial (env vars work, YAML loading incomplete) | 🟡 | P1 | Env vars fully supported including CACHE_DATA_DIR, CACHE_DATA_MAX_SIZE, CACHE_METADATA_TTL |
| Persistent inode store | N/A | Yes (bbolt) | ✅ | — | Crash recovery |
| Build-time version info | N/A | Yes (ldflags: version, commit, date) | ✅ | — | |
| S3 storage class support | Standard, IA, Intelligent-Tiering (no Glacier) | All (backend-dependent) | ✅ | — | |
| Custom S3 metadata preservation | No (stripped on file system changes) | Yes (preserved) | ✅ | — | **Our advantage** |

---

## Our Differentiators

Features and characteristics where we have advantages over AWS S3 Files:

| Differentiator | Details |
|---------------|---------|
| **Multi-backend support** | MinIO, AWS S3, Dell ObjectScale — not locked to any cloud vendor |
| **Self-hosted / air-gapped** | Deploy on-premises, in air-gapped environments, or any cloud |
| **Open source (Apache 2.0)** | Auditable, extensible, no license fees |
| **Custom S3 metadata preserved** | AWS strips user-defined metadata on file system changes; we preserve it |
| **Fine-grained cache control** | Separate TTLs for files (300s), directories (60s), negative entries (10s) — all configurable |
| **Prometheus-native observability** | Direct Prometheus integration vs CloudWatch-only on AWS |
| **Any NFS client** | Works with any Linux/macOS/Windows NFS client, not just AWS compute |
| **Single binary deployment** | Minimal operational overhead, no infrastructure to manage |
| **Negative caching** | Reduces S3 HEAD requests for non-existent paths (not documented in AWS) |
| **Adaptive prefetch tuning** | Explicit 3-tier prefetch (1→4→16 MB) based on sequential access detection |
| **Symlink support via S3 metadata** | AWS S3 Files supports symlinks natively; we emulate via marker objects — same end-user experience |

---

## Roadmap Summary

### P0 — Critical (next release)

| Item | Details |
|------|---------|
| ~~Data cache read-path integration~~ | ✅ Done — wired into `internal/s3fs/reader.go` with ETag coherency and LRU eviction. |

### P1 — High (next quarter)

| Item | Details |
|------|---------|
| ~~Intelligent read routing~~ | ✅ Done — small files (≤128KB) served from disk cache, large sequential reads streamed from S3. |
| ~~Read-after-write consistency~~ | ✅ Done — cache invalidated on write close; reads after writes see latest data. |
| ~~Configurable cache data expiration~~ | ✅ Done — via `CACHE_METADATA_TTL` and data cache TTL env vars. |
| Close-to-open consistency | NFS close-to-open consistency model for multi-client access (partial — subject to metadata cache TTL). |
| Dual-access consistency docs | Document consistency guarantees when accessing data via both NFS and direct S3. |
| YAML config file loading | Complete the config loading (env vars work, YAML parser needs finishing). |

### P2 — Medium (next 6 months)

| Item | Details |
|------|---------|
| ~~chmod/chown~~ | ✅ Done — update S3 `x-amz-meta-mode/uid/gid` headers via CopyObject with metadata replace. |
| ~~Symlink support~~ | ✅ Done — emulated via S3 marker objects with metadata. |
| ~~Rate limiting~~ | ✅ Done — token bucket limiter in `internal/ratelimit`. |
| ~~Truncate support~~ | ✅ Done — via empty object upload. |
| NFS traffic encryption | TLS for NFS traffic (stunnel wrapper or NFS-over-TLS). |
| NFSv4.2 protocol support | Depends on libnfs-go upstream adding v4.2 support. |
| Multipart upload | For files > 5GB, use S3 multipart upload API. |

### P3 — Low / Aspirational

| Item | Details |
|------|---------|
| Bidirectional sync | Cache ↔ S3 synchronization (import/export). Complex for self-hosted. |
| Horizontal scaling | Multi-instance with shared cache coherency or partitioned access. |
| NFS access points | Per-application mount points scoped to S3 prefixes. |
| Grafana dashboard templates | Pre-built dashboards for the Prometheus metrics we expose. |
| ML/Python framework testing | Validate compatibility with PyTorch DataLoader, HuggingFace datasets, etc. |
| Working set pre-warming | Lazy loading / pre-fetch of active working sets. |
| Random write optimization | In-memory page cache with write-back (significant complexity). |

---

## Methodology

- AWS features sourced from official docs and launch blog post (April 2026)
- Our features verified against source code in `internal/` packages
- Status reflects code state as of 2026-04-13
- **Partial (🟡)** means code exists but is not fully integrated or functional
- **Gap (⬜)** means AWS has the feature and we have no code for it
- Update this document when features are implemented or AWS releases changes
