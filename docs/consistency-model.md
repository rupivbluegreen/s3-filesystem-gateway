# Consistency Model

This document describes the consistency guarantees of the S3 filesystem gateway for workloads that access data via NFS, directly via the S3 API, or both simultaneously.

## Write Semantics (Write-on-Close)

The gateway uses a **write-on-close** model:

1. When an NFS client opens a file for writing, data is buffered to a local temporary file on the gateway host.
2. No data is written to S3 until the NFS client closes the file.
3. On close, the gateway uploads the full buffered content to S3 as a single `PutObject` operation.

**Implication:** Other clients (NFS or S3) cannot see a partially written file. The new version becomes atomically visible only after the close completes.

## Read-After-Write Consistency

| Scenario | Behavior |
|---|---|
| NFS write → NFS read (same client) | Immediate. Metadata cache is refreshed on close with the new file's attributes. |
| NFS write → NFS read (different client) | Visible within metadata cache TTL (file default: 300s). |
| NFS write → direct S3 read | Immediate. AWS S3 and MinIO both provide read-after-write consistency for `PutObject`. |

On close, the gateway explicitly invalidates and refreshes the metadata cache entry for the written key and its parent directory, so the writing client always sees the new data immediately on the next read.

## Close-to-Open Consistency

When client A closes a file it has written, other NFS clients that subsequently open the file will see the new data, subject to the metadata cache TTL:

- **File metadata TTL** (default 300s, `CACHE_METADATA_TTL` controls a shared value; see [Cache Behavior](#cache-behavior)): a client that has a cached stat for the old version of the file will continue to serve the cached attributes until the TTL expires.
- **Directory listing TTL** (default 60s): new files appear in directory listings within 60s of creation.

The writing client's close operation invalidates the cache immediately, so the writing client always sees fresh data. Other clients see stale data for at most the remaining TTL window.

To reduce the staleness window, lower `CACHE_METADATA_TTL`. Setting it to `0` disables metadata caching entirely (at the cost of increased S3 API traffic).

## Cache Behavior

### Metadata Cache (in-memory LRU)

| Parameter | Default | Env var |
|---|---|---|
| File TTL | 300s | `CACHE_METADATA_TTL` (shared) |
| Directory TTL | 60s | — |
| Negative (not-found) TTL | 10s | — |
| Max entries | 10,000 | — |
| Eviction interval | 30s | — |

The metadata cache stores file attributes (size, mtime, mode, ETag) and directory listings. It is shared across all NFS sessions on the same gateway instance.

**Invalidation rules:**
- A write (close) invalidates the exact S3 key and its parent directory listing.
- A rename invalidates both source and destination keys.
- A delete invalidates the removed key and its parent directory listing.

### Data Cache (disk-based LRU)

| Parameter | Default | Env var |
|---|---|---|
| Cache directory | `/var/cache/s3gw` | `CACHE_DATA_DIR` |
| Max total size | 10 GB | `CACHE_DATA_MAX_SIZE` |
| Coherency | ETag-based | — |
| Eviction interval | 30s | — |

The data cache stores full S3 object content on the gateway's local disk, keyed by `SHA256(s3Key + ETag)`. A cache hit requires the ETag to match the current object; if the object is replaced on S3, the ETag changes and the old cached data is bypassed.

On any write operation the gateway invalidates the data cache entry for the affected S3 key, ensuring subsequent reads fetch the new content.

## Dual Access: NFS and Direct S3

When data is accessed through both NFS (via the gateway) and the S3 API simultaneously, the following rules apply:

| Operation | Visibility |
|---|---|
| NFS write → direct S3 read | Immediate. The new object is in S3 immediately after the NFS client closes. |
| Direct S3 write → NFS read | Delayed by up to the metadata TTL (default 300s for files, 60s for dirs). |
| Direct S3 delete → NFS read | NFS may return a stale positive cache hit for up to 300s; after TTL expiry, `ENOENT` is returned. |

**The staleness window for S3-originated changes equals the metadata cache TTL.** The gateway has no mechanism to receive change notifications from S3, so it cannot proactively invalidate cached metadata when objects are modified outside of NFS.

**Recommended mitigations for mixed-access workloads:**

- Lower `CACHE_METADATA_TTL` (e.g., `30s` or `10s`) to reduce the staleness window.
- Coordinate writes so that S3-side changes are followed by a cache-busting NFS operation (e.g., a `stat` after a TTL flush) if strict consistency is required.
- Use the gateway as the single write path and treat direct S3 access as read-only where possible.

## Limitations

| Limitation | Details |
|---|---|
| No atomic rename | Rename is implemented as copy + delete. A crash between the two steps can leave both the source and destination present. |
| No mandatory locking | Advisory locks are session-scoped and not enforced across clients. Two NFS clients writing the same file concurrently will result in last-writer-wins (whichever client closes last wins). |
| No cross-client cache coherency | The gateway does not implement NFSv4 delegations or lease-based cache invalidation across sessions. |
| Write-once per open | Random writes within an open session are buffered locally but uploaded as a single object on close; there is no partial-object update on S3. |

## Recommendations by Use Case

| Use Case | Recommended TTL settings | Notes |
|---|---|---|
| Single-writer, many-reader (NFS only) | File: 300s (default), Dir: 60s (default) | Good performance; readers see writes within one TTL window. |
| Active mixed access (NFS + S3) | File: 10–30s, Dir: 10s | Higher S3 API traffic; tighter consistency. |
| Read-heavy, infrequent writes | File: 600s+, Dir: 120s | Best throughput; accept longer stale window after writes. |
| Compliance / audit data (write-once) | File: 3600s, Dir: 300s | Objects are never modified; long TTL is safe. |
| Real-time pipeline (strict consistency) | `CACHE_METADATA_TTL=0` | Disables metadata cache; every stat hits S3. Use only when necessary. |

Set `CACHE_METADATA_TTL` as a Go duration string (e.g., `30s`, `5m`, `1h`). Note that the env var currently controls a shared base value; the file TTL and directory TTL may differ from this value depending on internal defaults in the cache layer.
