// Copyright 2024 s3-filesystem-gateway contributors
// SPDX-License-Identifier: Apache-2.0

package s3fs

// Symlink metadata key stored on the S3 marker object.
const MetaKeySymlinkTarget = "Symlink-Target"

// isSymlink checks if an S3 object's metadata indicates it is a symlink.
func isSymlink(meta map[string]string) bool {
	_, ok := meta[MetaKeySymlinkTarget]
	return ok
}

// symlinkTarget returns the target path from a symlink marker object's metadata.
func symlinkTarget(meta map[string]string) string {
	return meta[MetaKeySymlinkTarget]
}
