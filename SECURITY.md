# Security Policy

## Reporting Vulnerabilities

If you discover a security vulnerability in s3-filesystem-gateway, please report it responsibly.

**Primary channel:** Use GitHub Security Advisories private vulnerability reporting: <https://github.com/rupivbluegreen/s3-filesystem-gateway/security/advisories/new>

**Secondary (fallback) channel:** `security@s3-filesystem-gateway.dev`

**Do not** open a public GitHub issue for security vulnerabilities.

Please include:
- A description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response Timeline

| Stage | Timeframe |
|-------|-----------|
| Acknowledgement | Within 48 hours |
| Initial assessment | Within 5 business days |
| Fix development | Within 30 days for critical issues |
| Public disclosure | After fix is released, coordinated with reporter |

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Previous minor release | Security fixes only |
| Older versions | No |

## Security Features

The gateway implements the following security measures:

### TLS for S3 Connections
TLS is enabled by default for all connections to S3-compatible backends. Disabling TLS requires explicit configuration and is not recommended for production use.

### Path Traversal Protection
All file paths are validated and sanitized to prevent directory traversal attacks (e.g., `../` sequences). Paths are resolved within the virtual filesystem root before being mapped to S3 keys.

### No Hardcoded Credentials
The gateway does not contain any hardcoded credentials. All AWS/S3 credentials are provided at runtime through environment variables, configuration files, or IAM roles.

### Non-Root Container Execution
The provided container image runs as a non-root user. The Dockerfile does not require or use root privileges at runtime.

### Restrictive File Permissions
Data directories are created with mode `0700`, ensuring only the owning user can read, write, or traverse them. This applies to local cache directories and any temporary storage.

### HTTP Server Timeouts (Slowloris Protection)
The health check and metrics HTTP servers are configured with read, write, and idle timeouts to mitigate slowloris and similar slow-connection denial-of-service attacks.

### Inode Counter Overflow Protection
The virtual inode allocator includes overflow detection to prevent inode number reuse or wrap-around, which could lead to file identity confusion.

## Known Limitations

### NFS Traffic Is Unencrypted
> NFS traffic between clients and the gateway is transmitted in plaintext in v0.1.0. **Native RFC 9289 RPC-with-TLS is planned for v0.3.0**, coupled with NFSv4.1/4.2 session support. Until then, deploy the gateway on a trusted network segment, or terminate TLS out-of-band via a Wireguard mesh / stunnel tunnel.

### No NFS Client Authentication
> The gateway uses AUTH_SYS (traditional UNIX UID/GID) for NFS authentication, which provides no cryptographic verification of client identity. When RFC 9289 TLS lands in v0.3.0, mutual-TLS with client certificates will be the recommended way to authenticate clients cryptographically. Kerberos (RPCSEC_GSS) is parked indefinitely -- TLS covers the same threat model with much less operational burden.

### Rename Is Not Atomic
Rename operations are implemented as copy-then-delete on S3, which is not atomic. A failure during rename may result in the object existing at both the old and new keys. This is an inherent limitation of S3's object storage model.

## Dependency Security

Dependencies are tracked in `go.mod` and `go.sum`. To check for known vulnerabilities:

```bash
go install golang.org/dl/govulncheck@latest
govulncheck ./...
```

Keep dependencies up to date by running:

```bash
go get -u ./...
go mod tidy
```

Review dependency updates for security advisories before upgrading in production.

## EU/GDPR Compliance Notes

- **No personal data collected or stored.** The gateway itself does not collect, process, or store any personal data. It acts as a transparent protocol translation layer between NFS clients and an S3-compatible storage backend.

- **Transparent proxy model.** Data classification, retention policies, and GDPR compliance for stored data are the responsibility of the S3 storage backend operator, not the gateway.

- **Credentials are never logged.** AWS access keys, secret keys, session tokens, and other credentials are excluded from all log output regardless of log level.

- **Cache data follows backend access controls.** Locally cached data (metadata and content) is subject to the same access restrictions as the S3 backend. Cache directories use restrictive file permissions (0700).

- **Cache retention is bounded.** Cached data is automatically evicted based on TTL expiration and maximum cache size limits. No data is retained indefinitely.

- **No telemetry or third-party data sharing.** The gateway does not send telemetry, analytics, crash reports, or any data to third parties. All operations are strictly between the NFS client, the gateway, and the configured S3 backend.

- **Open source and auditable.** The gateway is released under the Apache License 2.0. The complete source code is available for security audit and compliance review.
