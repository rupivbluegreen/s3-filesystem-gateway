package s3fs

import (
	"testing"
)

func TestIsSymlink(t *testing.T) {
	if isSymlink(nil) {
		t.Error("nil metadata should not be symlink")
	}
	if isSymlink(map[string]string{"Uid": "1000"}) {
		t.Error("regular metadata should not be symlink")
	}
	if !isSymlink(map[string]string{MetaKeySymlinkTarget: "/foo/bar"}) {
		t.Error("metadata with symlink target should be symlink")
	}
}

func TestSymlinkTarget(t *testing.T) {
	meta := map[string]string{MetaKeySymlinkTarget: "/foo/bar"}
	if got := symlinkTarget(meta); got != "/foo/bar" {
		t.Errorf("symlinkTarget() = %q, want /foo/bar", got)
	}
	if got := symlinkTarget(nil); got != "" {
		t.Errorf("symlinkTarget(nil) = %q, want empty", got)
	}
}
