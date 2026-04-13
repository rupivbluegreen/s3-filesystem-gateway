# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/rupivbluegreen/s3-filesystem-gateway/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rupivbluegreen/s3-filesystem-gateway/releases/tag/v0.1.0
