# Dell ObjectScale Compatibility

This guide explains how to configure the s3-filesystem-gateway for use with
Dell ObjectScale as the S3-compatible storage backend.

## Overview

Dell ObjectScale exposes an S3-compatible API, typically on port 9020 (HTTP) or
9021 (HTTPS). The gateway works with ObjectScale by using path-style addressing
and standard S3v4 signatures.

## Configuration

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `S3_ENDPOINT` | ObjectScale S3 API endpoint (e.g. `objectscale-host:9020`) | `localhost:9000` |
| `S3_ACCESS_KEY` | ObjectScale access key | `minioadmin` |
| `S3_SECRET_KEY` | ObjectScale secret key | `minioadmin` |
| `S3_BUCKET` | Target bucket name | `data` |
| `S3_REGION` | S3 region | `us-east-1` |
| `S3_USE_SSL` | Enable TLS (`true`/`false`) | `false` |
| `S3_PATH_STYLE` | Use path-style bucket addressing (`true`/`false`) | `true` |

### YAML Configuration

```yaml
s3:
  endpoint: objectscale-host:9020
  access_key: <your-access-key>
  secret_key: <your-secret-key>
  bucket: data
  region: us-east-1
  use_ssl: false
  path_style: true
```

### Docker Compose

An ObjectScale-specific compose override is provided:

```bash
export OBJECTSCALE_ENDPOINT=objectscale-host:9020
export OBJECTSCALE_ACCESS_KEY=<key>
export OBJECTSCALE_SECRET_KEY=<secret>
export OBJECTSCALE_BUCKET=data

docker compose \
  -f deployments/docker/docker-compose.yml \
  -f deployments/docker/docker-compose.objectscale.yml \
  up gateway
```

## Required ObjectScale Permissions

The IAM user or ObjectScale account used by the gateway needs the following S3
permissions on the target bucket:

- `s3:GetObject` -- read file contents
- `s3:PutObject` -- write files
- `s3:DeleteObject` -- delete files
- `s3:ListBucket` -- list directory contents
- `s3:GetBucketLocation` -- connection validation

Example bucket policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": "urn:osc:iam:::user/<username>"},
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:GetBucketLocation"
      ],
      "Resource": [
        "arn:aws:s3:::<bucket>",
        "arn:aws:s3:::<bucket>/*"
      ]
    }
  ]
}
```

## Known Differences from AWS S3

- **Path-style addressing required.** ObjectScale may not support virtual-host
  style bucket URLs. Always set `S3_PATH_STYLE=true`.
- **Signature versions.** ObjectScale supports both S3 v2 and v4 signatures.
  The gateway defaults to v4, which works in all tested ObjectScale versions.
- **Error codes.** Some error responses may differ slightly from AWS S3 (e.g.
  different XML error detail text). The gateway handles these gracefully.
- **Bucket versioning and object lock** are supported by ObjectScale but are not
  used by the gateway.
- **Custom metadata headers** work identically to AWS S3.

## Testing Procedure

1. Ensure the ObjectScale endpoint is reachable from the host running the
   gateway.

2. Verify with curl that the S3 API responds:
   ```bash
   curl -v http://objectscale-host:9020/
   ```

3. Start the gateway with ObjectScale configuration (see above).

4. Check the gateway logs for the backend detection message:
   ```
   S3 backend detected  backend=objectscale  endpoint=objectscale-host:9020
   ```

5. Mount the NFS export and verify file operations:
   ```bash
   sudo mount -t nfs4 -o vers=4.0 localhost:2049:/ /mnt/s3
   ls /mnt/s3
   echo "test" > /mnt/s3/hello.txt
   cat /mnt/s3/hello.txt
   rm /mnt/s3/hello.txt
   ```

## Troubleshooting

### TLS Certificate Errors

If ObjectScale uses a self-signed or internal CA certificate and you see TLS
errors:

- For testing, set `S3_USE_SSL=false` and use port 9020 (HTTP).
- For production, add the CA certificate to the gateway container's trust store
  (e.g. copy it to `/usr/local/share/ca-certificates/` and run
  `update-ca-certificates`).

### "Bucket does not exist" Error

- Confirm the bucket was created in ObjectScale before starting the gateway.
- Check that the access key has `s3:ListBucket` permission.
- Verify the endpoint and port are correct.

### Path-Style Addressing Failures

If you see DNS resolution errors or "bucket not found" errors with the correct
bucket name, ensure `S3_PATH_STYLE=true` is set. Virtual-host style addressing
requires DNS wildcards that ObjectScale environments typically do not configure.

### Signature Version Mismatch

If authentication fails with a `SignatureDoesNotMatch` error, the endpoint may
require v2 signatures. This is uncommon with modern ObjectScale versions but can
be configured in the YAML or by setting the `SignatureVersion` field in the
client config to `"v2"`.

### Connection Refused

- Verify the ObjectScale data endpoint is on port 9020 (HTTP) or 9021 (HTTPS),
  not the management console port.
- Check firewall rules between the gateway host and ObjectScale.
