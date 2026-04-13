# Contributing

Thanks for your interest in contributing to `s3-filesystem-gateway`. This document covers the logistics; for architecture and design decisions see [`CLAUDE.md`](./CLAUDE.md) at the repo root and the [`README.md`](./README.md).

## Code of Conduct

By participating you agree to abide by our [Code of Conduct](./CODE_OF_CONDUCT.md).

## Reporting bugs

Open an issue using the **Bug report** template at <https://github.com/rupivbluegreen/s3-filesystem-gateway/issues/new/choose>. Please fill in gateway version, S3 backend, Linux kernel version, mount flags, steps to reproduce, and (if possible) gateway logs with credentials redacted.

**Security vulnerabilities** — do **not** open a public issue. Use [GitHub Security Advisories private reporting](https://github.com/rupivbluegreen/s3-filesystem-gateway/security/advisories/new) as described in [`SECURITY.md`](./SECURITY.md).

## Proposing features

For anything larger than a typo fix or a small bug fix, please open a [GitHub Discussion](https://github.com/rupivbluegreen/s3-filesystem-gateway/discussions) first so we can agree on the approach before you write code. Small PRs (typos, doc fixes, obvious bugs) can be opened directly.

## Development setup

### Prerequisites

- **Go 1.25+** (the module declares `go 1.25`)
- **Docker** (for integration tests and local MinIO)
- **`golangci-lint`** (for `make lint`) — install via <https://golangci-lint.run/welcome/install/>
- **Linux** with `nfs-common` (for mounting the gateway locally)

### Alternative: containerised toolchain

If you don't want to install Go locally, all build and test steps can run inside a container:

```bash
docker run --rm -v "$PWD:/app" -w /app golang:1.25-alpine sh -c '
  apk add --no-cache git &&
  go vet ./... &&
  go test ./... -race -coverprofile=coverage.out -covermode=atomic
'
```

### Common commands

```bash
make build              # builds bin/s3nfsgw
make test               # unit tests
make lint               # golangci-lint run
make all                # fmt + vet + lint + test + build (run this before opening a PR)
make test-integration   # docker-based integration tests (MinIO + gateway + NFS client)
```

### Running the gateway locally

```bash
# Start gateway + bundled MinIO
docker compose -f deployments/docker/docker-compose.yml up

# In another terminal, mount NFS (v4.0 is required in v0.1.0)
sudo mount -t nfs4 -o vers=4.0,port=2049,nolock localhost:/ /mnt/s3
ls /mnt/s3
```

## Branching and commit style

- **Target branch:** `main`
- **Commit messages:** we use [Conventional Commits](https://www.conventionalcommits.org/). Prefixes we use: `feat`, `fix`, `docs`, `chore`, `test`, `refactor`, `ci`, `perf`, `build`.
- **Sign-off:** we use the [Developer Certificate of Origin (DCO)](https://developercertificate.org/). Sign commits with `git commit -s`. There is **no CLA** — sign-off is sufficient.
- **One logical change per PR.** Prefer small, focused PRs over large sweeping ones.

## Before opening a pull request

1. `make all` passes locally (or the containerised equivalent above).
2. New code has unit tests. Target coverage: keep or improve the existing baseline (~79%).
3. Add a `CHANGELOG.md` entry under the `[Unreleased]` section describing user-visible changes.
4. Update `README.md` or `CLAUDE.md` if user-facing behaviour or architecture changed.
5. Your PR description follows the [PR template](.github/PULL_REQUEST_TEMPLATE.md).

## Code style

- `gofmt` + `goimports` (run via `make fmt` or any IDE integration).
- `golangci-lint` config at [`.golangci.yml`](./.golangci.yml) — CI enforces this.
- Prefer standard-library solutions over adding new dependencies. When adding a dependency, explain the choice in the PR description.

## License

By contributing to this project you agree that your contributions will be licensed under the [Apache License 2.0](./LICENSE), the same license as the project.
