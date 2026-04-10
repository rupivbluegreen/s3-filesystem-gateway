package s3fs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestHandleStore(t *testing.T) *HandleStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewHandleStore(dbPath)
	if err != nil {
		t.Fatalf("NewHandleStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewHandleStore_CreatesDBAndRootInode(t *testing.T) {
	store := newTestHandleStore(t)

	// Root inode (1) should map to key ""
	key, ok := store.GetKey(rootInode)
	if !ok {
		t.Fatal("root inode not found")
	}
	if key != "" {
		t.Fatalf("root inode key = %q, want empty string", key)
	}

	// Reverse lookup
	inode := store.GetInode("")
	if inode != rootInode {
		t.Fatalf("GetInode(\"\") = %d, want %d", inode, rootInode)
	}
}

func TestGetOrCreateInode_NewKey(t *testing.T) {
	store := newTestHandleStore(t)

	inode, err := store.GetOrCreateInode("foo/bar.txt")
	if err != nil {
		t.Fatalf("GetOrCreateInode: %v", err)
	}
	if inode == 0 {
		t.Fatal("expected non-zero inode")
	}
	if inode == rootInode {
		t.Fatal("should not return root inode for non-root key")
	}
}

func TestGetOrCreateInode_SameKeyReturnsSameInode(t *testing.T) {
	store := newTestHandleStore(t)

	inode1, err := store.GetOrCreateInode("foo/bar.txt")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	inode2, err := store.GetOrCreateInode("foo/bar.txt")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if inode1 != inode2 {
		t.Fatalf("same key returned different inodes: %d vs %d", inode1, inode2)
	}
}

func TestGetOrCreateInode_Concurrent(t *testing.T) {
	store := newTestHandleStore(t)

	const goroutines = 50
	results := make([]uint64, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = store.GetOrCreateInode("concurrent-key")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
	}

	// All goroutines should get the same inode
	expected := results[0]
	for i, v := range results {
		if v != expected {
			t.Fatalf("goroutine %d got inode %d, want %d", i, v, expected)
		}
	}
}

func TestGetInode_UnknownKey(t *testing.T) {
	store := newTestHandleStore(t)

	inode := store.GetInode("nonexistent")
	if inode != 0 {
		t.Fatalf("GetInode(unknown) = %d, want 0", inode)
	}
}

func TestGetKey_KnownAndUnknown(t *testing.T) {
	store := newTestHandleStore(t)

	inode, _ := store.GetOrCreateInode("test-key")
	key, ok := store.GetKey(inode)
	if !ok || key != "test-key" {
		t.Fatalf("GetKey(%d) = (%q, %v), want (\"test-key\", true)", inode, key, ok)
	}

	key, ok = store.GetKey(999999)
	if ok {
		t.Fatalf("GetKey(unknown) = (%q, true), want (\"\", false)", key)
	}
}

func TestRemoveByKey(t *testing.T) {
	store := newTestHandleStore(t)

	inode, _ := store.GetOrCreateInode("to-remove")

	if err := store.RemoveByKey("to-remove"); err != nil {
		t.Fatalf("RemoveByKey: %v", err)
	}

	if got := store.GetInode("to-remove"); got != 0 {
		t.Fatalf("after remove, GetInode = %d, want 0", got)
	}

	if _, ok := store.GetKey(inode); ok {
		t.Fatal("after remove, GetKey should return false")
	}
}

func TestRemoveByKey_NonExistent(t *testing.T) {
	store := newTestHandleStore(t)

	// Should be a no-op, no error
	if err := store.RemoveByKey("does-not-exist"); err != nil {
		t.Fatalf("RemoveByKey(non-existent) = %v, want nil", err)
	}
}

func TestRenameKey(t *testing.T) {
	store := newTestHandleStore(t)

	inode, _ := store.GetOrCreateInode("old-key")

	if err := store.RenameKey("old-key", "new-key"); err != nil {
		t.Fatalf("RenameKey: %v", err)
	}

	// Old key should be gone
	if got := store.GetInode("old-key"); got != 0 {
		t.Fatalf("old key still maps to inode %d", got)
	}

	// New key should map to same inode
	if got := store.GetInode("new-key"); got != inode {
		t.Fatalf("new key maps to %d, want %d", got, inode)
	}

	// Reverse lookup
	key, ok := store.GetKey(inode)
	if !ok || key != "new-key" {
		t.Fatalf("GetKey(%d) = (%q, %v), want (\"new-key\", true)", inode, key, ok)
	}
}

func TestRenameKey_NonExistent(t *testing.T) {
	store := newTestHandleStore(t)

	err := store.RenameKey("no-such-key", "new-key")
	if err != os.ErrNotExist {
		t.Fatalf("RenameKey(non-existent) = %v, want os.ErrNotExist", err)
	}
}

func TestInodeToHandle_HandleToInode_Roundtrip(t *testing.T) {
	for _, inode := range []uint64{1, 2, 42, 1<<32 + 7, 1<<63 - 1} {
		handle := InodeToHandle(inode)
		if len(handle) != 8 {
			t.Fatalf("handle length = %d, want 8", len(handle))
		}

		got, err := HandleToInode(handle)
		if err != nil {
			t.Fatalf("HandleToInode: %v", err)
		}
		if got != inode {
			t.Fatalf("roundtrip failed: got %d, want %d", got, inode)
		}
	}
}

func TestHandleToInode_ShortHandle(t *testing.T) {
	_, err := HandleToInode([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for short handle")
	}
}

func TestRootHandle(t *testing.T) {
	h := RootHandle()
	if len(h) != 8 {
		t.Fatalf("RootHandle length = %d, want 8", len(h))
	}

	inode, err := HandleToInode(h)
	if err != nil {
		t.Fatalf("HandleToInode(RootHandle): %v", err)
	}
	if inode != rootInode {
		t.Fatalf("RootHandle inode = %d, want %d", inode, rootInode)
	}
}

func TestHandleStore_Close(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "close-test.db")
	store, err := NewHandleStore(dbPath)
	if err != nil {
		t.Fatalf("NewHandleStore: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestHandleStore_Persistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.db")

	// Open, add entries, close
	store, err := NewHandleStore(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	inode1, _ := store.GetOrCreateInode("persist/file1.txt")
	inode2, _ := store.GetOrCreateInode("persist/file2.txt")

	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	// Reopen and verify
	store2, err := NewHandleStore(dbPath)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer store2.Close()

	if got := store2.GetInode("persist/file1.txt"); got != inode1 {
		t.Fatalf("after reopen, file1 inode = %d, want %d", got, inode1)
	}
	if got := store2.GetInode("persist/file2.txt"); got != inode2 {
		t.Fatalf("after reopen, file2 inode = %d, want %d", got, inode2)
	}

	// New allocations should not collide
	inode3, _ := store2.GetOrCreateInode("persist/file3.txt")
	if inode3 == inode1 || inode3 == inode2 {
		t.Fatalf("new inode %d collides with existing", inode3)
	}
}
