# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - native NFSv4.1/v4.2 + RFC 9289 RPC-with-TLS

### Added
- **NFSv4.1 session support.** Full RFC 8881 §18 implementation of `EXCHANGE_ID`, `CREATE_SESSION`, `SEQUENCE`, `DESTROY_SESSION`, `DESTROY_CLIENTID`, `FREE_STATEID`, `RECLAIM_COMPLETE`, and `SECINFO_NO_NAME` with a backend-wide `SessionRegistry` shared across connections. Compound dispatcher enforces SEQUENCE-first for minorversion ≥ 1. Modern Linux kernels can now mount with the default (no `vers=4.0` flag).
- **NFSv4.2 minorversion + selected feature ops** (RFC 7862, RFC 8276):
  - `SEEK` — "no-holes" stub appropriate for flat object storage; `SEEK_HOLE` returns EOF, `SEEK_DATA` echoes the offset or returns `NXIO`.
  - `COPY` — synchronous full-object server-side copy that maps `cp a b` to a single S3 `CopyObject` request via the new `fs.CopyCapable` optional interface. Partial-range and async copies cleanly return `NFS4ERR_NOTSUPP` so the kernel falls back to client-side `READ`+`WRITE`.
  - `GETXATTR` / `SETXATTR` / `LISTXATTRS` / `REMOVEXATTR` — RFC 8276 user xattrs mapped to S3 user-metadata via a new `fs.XattrCapable` optional interface. Names are hex-encoded so case survives MinIO/S3 header case-folding; values are base64-encoded so arbitrary binary survives the ASCII-only S3 header round-trip.
  - `xattr_support` (attr 82) and `ACCESS4_XAREAD` / `XAWRITE` / `XALIST` advertised so the Linux client actually issues xattr ops instead of short-circuiting to `EOPNOTSUPP`.
  - `ALLOCATE` / `DEALLOCATE` decode their args and return `NFS4ERR_NOTSUPP` so the compound stays aligned and `fallocate` fails cleanly.
- **RFC 9289 in-band STARTTLS upgrade.** A client probing with `cred.flavor = AUTH_TLS` triggers the server to reply with `AUTH_TLS` in the verifier and immediately wrap the same TCP connection in `tls.Server`, then continue all subsequent NFS RPCs over the encrypted transport. Linux 6.5+ clients opt in with `mount -o xprtsec=tls,vers=4.2`. Plaintext clients on the same port keep working unchanged.
- **TLS configuration:** `NFS_TLS_ENABLE`, `NFS_TLS_CERT_FILE`, `NFS_TLS_KEY_FILE`, `NFS_TLS_CLIENT_CA_FILE` (mTLS), `NFS_TLS_MIN_VERSION` (default `1.3`, `1.2` supported). Validation rejects misconfigurations at startup.
- **Slot reply cache** (RFC 8881 §2.10.6.2). The compound dispatcher caches the body of every successful compound in the active session's slot, and short-circuits SEQUENCE retransmits with the cached bytes so already-executed ops (e.g. an `OPEN` that allocated state) return byte-identical results without re-execution.
- `EXCLUSIVE4_1` open create-mode (RFC 5661 §18.16) — fixes `cp a b` on Linux 6.x at v4.2, which sends `O_EXCL` creates through this mode instead of `GUARDED4`.
- `deployments/docker/docker-compose.quickstart.tls.yml` — consumer quickstart with self-signed cert generated at first run, and `deployments/docker/docker-compose.test.tls.yml` — integration test rig that drives `xprtsec=tls` against a CI host with `ktls-utils` installed.

### Changed
- libnfs-go fork at `github.com/rupivbluegreen/libnfs-go` (replace directive in `go.mod`) — Stream A/B/C all live here; will be unpinned once the fork is tagged.
- README "Quick Start" mount example uses the kernel default (v4.2) instead of `-o vers=4.0`. New "Encrypted NFS (RFC 9289)" subsection documents the TLS quickstart, client-side `ktls-utils` setup, and the env vars.
- SECURITY.md now reflects the v0.3.0 reality: in-transit encryption is available via TLS (replacing the v0.1.0 "use stunnel or a VPN" guidance), and mTLS is the recommended client authentication path (replacing the "Kerberos planned" line).
- `xdr.Writer` gained a `Marshaler` interface mirroring the existing `Unmarshaler` on `xdr.Reader`, so types whose wire format can't be expressed by reflection (e.g. the empty-only `netloc4<>` field of `COPY4args`) can hook their own encoder.
- `compound.go` default case now returns `NFS4ERR_NOTSUPP` in a well-formed reply instead of `NFS4ERR_OP_ILLEGAL` and a truncated wire — required for v4.2 ops we don't implement to fail gracefully.
- v4.1 stateid lookup uses `Other[0]` as the stable open-file identifier (kernel increments `SeqId` per op at v4.1+), refactored into a shared `lookupOpenFile` helper.

### Fixed
- XDR reflection-based encoder skipped nil slices entirely instead of emitting the length prefix, corrupting any field that followed. Now always emits `uint32(0)` for nil/empty slices.
- `state_protect4_a`, `state_protect4_r`, and `callback_sec_parms4` now have custom `XdrUnmarshal` methods because reflection cannot express discriminated unions; without these, the very first `EXCHANGE_ID` from a v4.1 client failed to decode.
- `OPEN` decoder now handles `CLAIM_FH` / `CLAIM_DELEG_CUR_FH` / `CLAIM_DELEG_PREV_FH` (v4.1+ claim types) so kernels can open files by filehandle.

### Out of scope
- Kerberos / RPCSEC_GSS — parked indefinitely; in-band TLS + mTLS covers the same threat model.
- pNFS layouts — incompatible with single-node S3.
- Async COPY / CLONE / READ_PLUS / WRITE_SAME — return `NFS4ERR_NOTSUPP`; kernel emulates client-side.

### Known limitations
- Live `mount -o xprtsec=tls` requires a Linux 6.5+ client kernel built with `CONFIG_HANDSHAKE` plus the `ktls-utils` userspace daemon (`tlshd`). Ubuntu 24.04's stock 6.8 kernel ships the `xprtsec=tls` mount option but not the userspace upcall plumbing — install `ktls-utils` and use a kernel that has `CONFIG_HANDSHAKE` enabled.

## [0.2.0] - prior OSS hygiene work

### Added
- Community files for professional OSS hygiene: `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md`, `CHANGELOG.md`, `CODEOWNERS`, issue/PR templates.
- CI workflow (`.github/workflows/ci.yml`) running `go vet`, `go test -race -cover`, `golangci-lint`, `go build`, and a Docker build smoke on every pull request and push to `main`.
- OpenSSF Scorecard workflow (`.github/workflows/scorecard.yml`) for continuous supply-chain scoring.
- Dependabot configuration for weekly `gomod`, `github-actions`, and `docker` updates.
- `golangci-lint` config (`.golangci.yml`) enabling the standard linter set plus `misspell`, `unconvert`, and `goimports` with the local module prefix.
- README badges for CI status, license, latest release, Go Report Card, OpenSSF Scorecard, and published image registries.

### Changed
- `SECURITY.md` vulnerability reporting now points at GitHub Security Advisories private reporting as the primary channel (email kept as fallback).
- `SECURITY.md` known-limitations section updated to reflect the P0-P3 work already shipped in v0.1.0: rate limiter and chmod/chown are features now, not limitations.
- Renamed `internal/s3fs.chunkReader.Seek` to `SeekTo` to unblock `go vet` (the previous name collided with `io.Seeker`'s signature).

## [0.1.0] - 2026-04-13

First public release. NFSv4.0-only. Serves S3-compatible object storage as an NFSv4 filesystem.

### Added
- NFSv4.0 server backed by any S3-compatible storage (MinIO, AWS S3, Dell ObjectScale), built on `libnfs-go`.
- Multi-arch (`linux/amd64` + `linux/arm64`) Docker images published to GHCR (`ghcr.io/rupivbluegreen/s3-filesystem-gateway`) and Docker Hub (`docker.io/vipurkumar/s3-filesystem-gateway`) on every `v*` git tag.
- `deployments/docker/docker-compose.quickstart.yml` — one-shot dev environment with bundled MinIO for consumer testing.
- Metadata cache (in-memory LRU, bbolt-backed inode persistence, negative caching, directory listing cache).
- Disk-based data cache with ETag coherency, TTL expiry, and adaptive prefetch (1 MB → 4 MB → 16 MB).
- Full POSIX operations: create, mkdir, remove, rename (copy+delete), stat, readdir, chmod, chown, truncate, symlinks.
- Read-after-write consistency via cache refresh on write close.
- Token-bucket rate limiter for S3 request shaping.
- Prometheus metrics, `/health` and `/ready` endpoints, Grafana dashboard template.
- Structured JSON logging via `log/slog`; graceful SIGINT/SIGTERM shutdown with 30 s timeout.
- OCI image labels stamped with version / commit / build date via `-ldflags`.
- Comprehensive unit test suite (79.3 % total coverage at release time).

### Fixed
- NFSv4 auth handler was nil, causing SIGSEGV in `libnfs-go/server.Muxv4.Authenticate` on the first client compound — now wires a permissive AUTH_NONE + AUTH_SYS handler (rc3).
- `/ready` endpoint could panic if `NewHealthServer` was called with a nil checker — now substitutes a no-op checker (rc3).
- `OPEN` with `CREATE` did not persist to S3 until `Close`, so subsequent `GETATTR`/`ACCESS` compounds failed with `ENOENT` and hung the client. Now writes an empty placeholder on create and uses a unified read-write `s3WritableFile` with lazy download-modify-upload semantics (rc3).
- Dockerfile did not create `/var/lib/s3nfsgw` or `/var/cache/s3gw` with correct ownership — first run under the quickstart compose would fail with "permission denied" on the named volume mount (rc2).
- Bumped builder image from `golang:1.22-alpine` to `golang:1.25-alpine` to match `go.mod` `go 1.25`.

### Known limitations
- NFSv4.0 only. Modern Linux kernel clients must mount with explicit `-o vers=4.0`. NFSv4.1 / 4.2 session support planned for v0.3.0.
- Plaintext NFS traffic only. RFC 9289 RPC-with-TLS native support planned for v0.3.0 (coupled with NFSv4.2 session support).
- Rename is not atomic (implemented as copy + delete) due to S3's object-storage semantics.

[Unreleased]: https://github.com/rupivbluegreen/s3-filesystem-gateway/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/rupivbluegreen/s3-filesystem-gateway/compare/v0.1.0...v0.3.0
[0.1.0]: https://github.com/rupivbluegreen/s3-filesystem-gateway/releases/tag/v0.1.0
