package cache

import (
	"bytes"
	"container/list"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func tempDataCache(t *testing.T, maxSize int64) *DataCache {
	t.Helper()
	dir := t.TempDir()
	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: maxSize,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	t.Cleanup(dc.Stop)
	return dc
}

func TestDataCache_PutGet(t *testing.T) {
	dc := tempDataCache(t, 1<<20) // 1 MB

	data := []byte("hello, world")
	err := dc.Put("my/key.txt", "etag1", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, ok := dc.Get("my/key.txt", "etag1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestDataCache_Miss(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	// Miss on empty cache.
	_, ok := dc.Get("no/such/key", "etag")
	if ok {
		t.Fatal("expected cache miss on empty cache")
	}

	// Put one key, miss on different etag.
	data := []byte("data")
	if err := dc.Put("key", "etag1", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, ok = dc.Get("key", "etag2")
	if ok {
		t.Fatal("expected cache miss for different etag")
	}
}

func TestDataCache_Eviction(t *testing.T) {
	// Max size = 100 bytes.
	dc := tempDataCache(t, 100)

	// Put 3 entries of 40 bytes each (total 120 > 100).
	for i, key := range []string{"a", "b", "c"} {
		data := bytes.Repeat([]byte{byte('A' + i)}, 40)
		if err := dc.Put(key, "e", bytes.NewReader(data), 40); err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}

	stats := dc.Stats()
	if stats.CurrentSize > 100 {
		t.Fatalf("cache size %d exceeds max 100", stats.CurrentSize)
	}

	// The oldest entry ("a") should have been evicted.
	_, ok := dc.Get("a", "e")
	if ok {
		t.Fatal("expected entry 'a' to be evicted")
	}

	// "b" or "c" should still be present.
	_, ok = dc.Get("c", "e")
	if !ok {
		t.Fatal("expected entry 'c' to still be cached")
	}
}

func TestDataCache_Invalidate(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	data := []byte("payload")
	// Put two versions of same key.
	if err := dc.Put("obj", "v1", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := dc.Put("obj", "v2", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	dc.Invalidate("obj")

	if _, ok := dc.Get("obj", "v1"); ok {
		t.Fatal("expected v1 to be invalidated")
	}
	if _, ok := dc.Get("obj", "v2"); ok {
		t.Fatal("expected v2 to be invalidated")
	}

	stats := dc.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("expected 0 entries after invalidation, got %d", stats.EntryCount)
	}
}

func TestDataCache_Stats(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	data := []byte("stats-test")
	if err := dc.Put("k", "e", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// One hit.
	rc, ok := dc.Get("k", "e")
	if !ok {
		t.Fatal("expected hit")
	}
	rc.Close()

	// One miss.
	dc.Get("k", "wrong")

	stats := dc.Stats()
	if stats.Hits != 1 {
		t.Fatalf("hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("misses = %d, want 1", stats.Misses)
	}
	if stats.EntryCount != 1 {
		t.Fatalf("entry count = %d, want 1", stats.EntryCount)
	}
	if stats.CurrentSize != int64(len(data)) {
		t.Fatalf("current size = %d, want %d", stats.CurrentSize, len(data))
	}
}

func TestCacheKey(t *testing.T) {
	k1 := cacheKey("a", "b")
	k2 := cacheKey("a", "c")
	if k1 == k2 {
		t.Fatal("different etags should produce different keys")
	}
	if len(k1) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(k1))
	}
}

func TestCachePath_Sharding(t *testing.T) {
	dc := tempDataCache(t, 1<<20)
	key := cacheKey("test", "etag")
	path := dc.cachePath(key)

	// Path should contain the shard directory.
	if !strings.Contains(path, filepath.Join(key[:2], key)) {
		t.Fatalf("path %q does not contain expected shard structure", path)
	}
}

func TestDataCache_ScanDir(t *testing.T) {
	dir := t.TempDir()

	// Pre-populate the cache directory.
	key := cacheKey("scan/test", "etag")
	shardDir := filepath.Join(dir, key[:2])
	os.MkdirAll(shardDir, 0o755)
	os.WriteFile(filepath.Join(shardDir, key), []byte("scanned"), 0o644)

	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	stats := dc.Stats()
	if stats.EntryCount != 1 {
		t.Fatalf("expected 1 entry after scan, got %d", stats.EntryCount)
	}
	if stats.CurrentSize != 7 { // len("scanned")
		t.Fatalf("expected size 7, got %d", stats.CurrentSize)
	}
}

func TestDefaultDataCacheConfig(t *testing.T) {
	cfg := DefaultDataCacheConfig()
	if cfg.Dir != "/var/cache/s3gw" {
		t.Errorf("Dir = %q, want /var/cache/s3gw", cfg.Dir)
	}
	if cfg.MaxSize != 10*1024*1024*1024 {
		t.Errorf("MaxSize = %d, want 10GB", cfg.MaxSize)
	}
}

func TestNewDataCacheDefaults(t *testing.T) {
	dir := t.TempDir()
	// Empty Dir and zero MaxSize should get defaults, but we override Dir
	// to avoid writing to /var/cache/s3gw.
	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: -1, // should become default
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	if dc.config.MaxSize != 10*1024*1024*1024 {
		t.Errorf("MaxSize = %d, want default 10GB", dc.config.MaxSize)
	}
}

func TestDataCache_GetFileDisappeared(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	data := []byte("disappearing data")
	if err := dc.Put("vanish", "e1", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Delete the file from disk behind the cache's back.
	key := cacheKey("vanish", "e1")
	path := dc.cachePath(key)
	os.Remove(path)

	// Get should return miss and clean up the index.
	_, ok := dc.Get("vanish", "e1")
	if ok {
		t.Fatal("expected cache miss when file disappeared from disk")
	}

	stats := dc.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("expected 0 entries after file disappeared, got %d", stats.EntryCount)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestDataCache_PutUpdateExisting(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	data1 := []byte("version1")
	data2 := []byte("version2-longer")

	if err := dc.Put("obj", "etag", bytes.NewReader(data1), int64(len(data1))); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := dc.Put("obj", "etag", bytes.NewReader(data2), int64(len(data2))); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	stats := dc.Stats()
	if stats.EntryCount != 1 {
		t.Fatalf("expected 1 entry, got %d", stats.EntryCount)
	}
	if stats.CurrentSize != int64(len(data2)) {
		t.Fatalf("expected size %d, got %d", len(data2), stats.CurrentSize)
	}

	// Verify we read back the updated data.
	rc, ok := dc.Get("obj", "etag")
	if !ok {
		t.Fatal("expected cache hit")
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, data2) {
		t.Fatalf("got %q, want %q", got, data2)
	}
}

func TestDataCache_PutWriteFailure(t *testing.T) {
	dir := t.TempDir()
	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	// Place a regular file where the shard directory should be, so MkdirAll fails.
	key := cacheKey("fail", "e1")
	shardPath := filepath.Join(dir, key[:2])
	os.WriteFile(shardPath, []byte("blocker"), 0o644)

	err = dc.Put("fail", "e1", bytes.NewReader([]byte("data")), 4)
	if err == nil {
		t.Fatal("expected error when shard dir creation fails")
	}
	if !strings.Contains(err.Error(), "create shard dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataCache_PutZeroSize(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	err := dc.Put("empty", "e1", bytes.NewReader(nil), 0)
	if err != nil {
		t.Fatalf("Put zero-size: %v", err)
	}

	rc, ok := dc.Get("empty", "e1")
	if !ok {
		t.Fatal("expected cache hit for zero-size entry")
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if len(got) != 0 {
		t.Fatalf("expected empty data, got %d bytes", len(got))
	}
}

func TestDataCache_EvictNoOp(t *testing.T) {
	dc := tempDataCache(t, 1<<20) // 1 MB limit

	data := []byte("small")
	if err := dc.Put("k", "e", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Manually trigger evict - should be a no-op since we're under limit.
	dc.evict()

	stats := dc.Stats()
	if stats.EntryCount != 1 {
		t.Fatalf("expected 1 entry after no-op evict, got %d", stats.EntryCount)
	}
}

func TestDataCache_ScanDirEdgeCases(t *testing.T) {
	dir := t.TempDir()

	// Create a non-shard file at root (should be skipped).
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644)

	// Create a directory with wrong name length (should be skipped).
	os.MkdirAll(filepath.Join(dir, "abc"), 0o755)
	os.WriteFile(filepath.Join(dir, "abc", "file"), []byte("data"), 0o644)

	// Create a valid shard with a tmp file (should be skipped).
	key := cacheKey("test", "e1")
	shardDir := filepath.Join(dir, key[:2])
	os.MkdirAll(shardDir, 0o755)
	os.WriteFile(filepath.Join(shardDir, ".tmp-partial"), []byte("temp"), 0o644)

	// Create a valid shard with a subdirectory (should be skipped).
	os.MkdirAll(filepath.Join(shardDir, "subdir"), 0o755)

	// Create a valid file.
	os.WriteFile(filepath.Join(shardDir, key), []byte("valid"), 0o644)

	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	stats := dc.Stats()
	if stats.EntryCount != 1 {
		t.Fatalf("expected 1 entry after scan with edge cases, got %d", stats.EntryCount)
	}
}

func TestDataCache_ScanDirSkipsNonShardEntries(t *testing.T) {
	dir := t.TempDir()

	// Create a regular file at the cache root level (not a shard dir).
	os.WriteFile(filepath.Join(dir, "not-a-shard"), []byte("data"), 0o644)

	// Create a directory with 3-char name (wrong length for shard).
	os.MkdirAll(filepath.Join(dir, "abc"), 0o755)

	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	stats := dc.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("expected 0 entries, got %d", stats.EntryCount)
	}
}

func TestDataCache_ConcurrentPutGet(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a'+i)) + "key"
			data := bytes.Repeat([]byte{byte(i)}, 100)
			if err := dc.Put(key, "e", bytes.NewReader(data), 100); err != nil {
				t.Errorf("Put %s: %v", key, err)
			}
			if rc, ok := dc.Get(key, "e"); ok {
				rc.Close()
			}
		}(i)
	}
	wg.Wait()

	stats := dc.Stats()
	if stats.EntryCount != 10 {
		t.Fatalf("expected 10 entries, got %d", stats.EntryCount)
	}
}

func TestDataCache_GetAfterStop(t *testing.T) {
	dir := t.TempDir()
	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}

	data := []byte("persist")
	if err := dc.Put("k", "e", bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	dc.Stop()

	// Data should still be readable from disk via a new cache instance.
	dc2, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache after stop: %v", err)
	}
	defer dc2.Stop()

	// The scanDir picks up the file; we can read it via the key.
	stats := dc2.Stats()
	if stats.EntryCount != 1 {
		t.Fatalf("expected 1 entry after restart, got %d", stats.EntryCount)
	}
}

func TestCacheKey_DifferentInputs(t *testing.T) {
	k1 := cacheKey("key1", "etag")
	k2 := cacheKey("key2", "etag")
	if k1 == k2 {
		t.Fatal("different s3Keys should produce different keys")
	}
	k3 := cacheKey("key", "etag1")
	k4 := cacheKey("key", "etag2")
	if k3 == k4 {
		t.Fatal("different etags should produce different keys")
	}
	// Same inputs produce same key.
	k5 := cacheKey("same", "same")
	k6 := cacheKey("same", "same")
	if k5 != k6 {
		t.Fatal("same inputs should produce same key")
	}
}

func TestDataCache_EvictionLoop(t *testing.T) {
	dir := t.TempDir()
	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 50, // very small
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	// Put data that fits within limit.
	data := bytes.Repeat([]byte("x"), 40)
	if err := dc.Put("k", "e", bytes.NewReader(data), 40); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Manually trigger evict via the loop mechanism (the loop uses dc.evict()).
	dc.evict()

	stats := dc.Stats()
	if stats.CurrentSize > 50 {
		t.Fatalf("cache size %d exceeds max 50", stats.CurrentSize)
	}
}

func TestDataCache_PutReaderError(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	// Use a reader that returns an error.
	err := dc.Put("err", "e1", &errReader{}, 100)
	if err == nil {
		t.Fatal("expected error from failing reader")
	}
	if !strings.Contains(err.Error(), "write cache file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type errReader struct{}

func (e *errReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestDataCache_ScanDirEmpty(t *testing.T) {
	dir := t.TempDir()

	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	stats := dc.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("expected 0 entries on empty dir, got %d", stats.EntryCount)
	}
}

// Ensure evictionLoop ticker path is covered by using a short interval.
func TestDataCache_EvictionLoopTicker(t *testing.T) {
	dir := t.TempDir()
	dc, err := NewDataCache(DataCacheConfig{
		Dir:              dir,
		MaxSize:          1 << 20,
		EvictionInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	// Let the ticker fire at least once.
	time.Sleep(50 * time.Millisecond)
	dc.Stop()
}

func TestNewDataCache_EmptyDir(t *testing.T) {
	// Empty Dir should use the default, but that requires /var/cache/s3gw write access.
	// Instead, test that an empty string triggers the default path assignment.
	dc, err := NewDataCache(DataCacheConfig{
		Dir:     "",
		MaxSize: 1 << 20,
	})
	if err != nil {
		// May fail if /var/cache/s3gw is not writable - that's fine, we just
		// need to verify the default was set. If it fails, verify it tried the default path.
		if !strings.Contains(err.Error(), defaultDataCacheDir) && !strings.Contains(err.Error(), "/var/cache/s3gw") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	defer dc.Stop()
	if dc.config.Dir != defaultDataCacheDir {
		t.Errorf("Dir = %q, want %q", dc.config.Dir, defaultDataCacheDir)
	}
}

func TestNewDataCache_MkdirAllFailure(t *testing.T) {
	// Use a path under a file to make MkdirAll fail.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0o644)

	_, err := NewDataCache(DataCacheConfig{
		Dir:     filepath.Join(blocker, "subdir"),
		MaxSize: 1 << 20,
	})
	if err == nil {
		t.Fatal("expected error when cache dir creation fails")
	}
	if !strings.Contains(err.Error(), "create cache dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataCache_PutCreateTempFailure(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	// Override osCreateTemp to simulate failure.
	origCreateTemp := osCreateTemp
	osCreateTemp = func(dir, pattern string) (*os.File, error) {
		return nil, fmt.Errorf("injected create temp error")
	}
	defer func() { osCreateTemp = origCreateTemp }()

	err := dc.Put("tempfail", "e1", bytes.NewReader([]byte("data")), 4)
	if err == nil {
		t.Fatal("expected error when CreateTemp fails")
	}
	if !strings.Contains(err.Error(), "create temp file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataCache_PutCloseError(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	// Override afterCopy to close the fd before Put calls Close(), causing
	// Close() to fail with "file already closed".
	origAfterCopy := afterCopy
	afterCopy = func(f *os.File) {
		f.Close() // pre-close so the real Close() fails
	}
	defer func() { afterCopy = origAfterCopy }()

	err := dc.Put("closetest", "e1", bytes.NewReader([]byte("hello")), 5)
	if err == nil {
		t.Fatal("expected error when Close() fails")
	}
	if !strings.Contains(err.Error(), "write cache file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataCache_PutRenameFailure(t *testing.T) {
	dc := tempDataCache(t, 1<<20)

	// We need rename to fail. Create the target path as a directory.
	key := cacheKey("renamefail", "e1")
	shardDir := filepath.Join(dc.config.Dir, key[:2])
	os.MkdirAll(shardDir, 0o755)
	// Create a directory where the final file should go.
	os.MkdirAll(filepath.Join(shardDir, key), 0o755)
	// Put a file inside so the directory isn't empty (rename over non-empty dir fails).
	os.WriteFile(filepath.Join(shardDir, key, "blocker"), []byte("x"), 0o644)

	err := dc.Put("renamefail", "e1", bytes.NewReader([]byte("data")), 4)
	if err == nil {
		t.Fatal("expected error when rename fails")
	}
	if !strings.Contains(err.Error(), "rename cache file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDataCache_ScanDirFileInfoError(t *testing.T) {
	dir := t.TempDir()
	shardDir := filepath.Join(dir, "ab")
	os.MkdirAll(shardDir, 0o755)

	// Override osReadDir to return an entry whose Info() fails for the shard.
	origReadDir := osReadDir
	callCount := 0
	osReadDir = func(name string) ([]os.DirEntry, error) {
		callCount++
		if callCount == 1 {
			// First call: read the cache root normally.
			return origReadDir(name)
		}
		// Second call: reading shard dir - return a fake entry whose Info() fails.
		return []os.DirEntry{&brokenInfoEntry{name: "fakefile"}}, nil
	}
	defer func() { osReadDir = origReadDir }()

	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	stats := dc.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("expected 0 entries with broken Info(), got %d", stats.EntryCount)
	}
}

// brokenInfoEntry is an os.DirEntry whose Info() always returns an error.
type brokenInfoEntry struct {
	name string
}

func (b *brokenInfoEntry) Name() string               { return b.name }
func (b *brokenInfoEntry) IsDir() bool                { return false }
func (b *brokenInfoEntry) Type() os.FileMode          { return 0 }
func (b *brokenInfoEntry) Info() (os.FileInfo, error) { return nil, fmt.Errorf("injected info error") }

func TestDataCache_ScanDirNonExistent(t *testing.T) {
	// Manually create a DataCache with a non-existent scanDir path.
	// scanDir should handle ReadDir error gracefully.
	dc := &DataCache{
		config: DataCacheConfig{Dir: "/nonexistent/path/for/test"},
		items:  make(map[string]*list.Element),
		order:  list.New(),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	// Should not panic.
	dc.scanDir()
	if dc.order.Len() != 0 {
		t.Fatalf("expected 0 entries, got %d", dc.order.Len())
	}
}

func TestDataCache_ScanDirReadDirShardError(t *testing.T) {
	dir := t.TempDir()
	shardDir := filepath.Join(dir, "ab")
	os.MkdirAll(shardDir, 0o755)
	os.WriteFile(filepath.Join(shardDir, "somefile"), []byte("data"), 0o644)

	// Override osReadDir to fail on the shard directory.
	origReadDir := osReadDir
	callCount := 0
	osReadDir = func(name string) ([]os.DirEntry, error) {
		callCount++
		if callCount == 1 {
			// First call: read the cache root normally.
			return origReadDir(name)
		}
		// Second call: reading shard dir - fail.
		return nil, fmt.Errorf("injected shard read error")
	}
	defer func() { osReadDir = origReadDir }()

	dc, err := NewDataCache(DataCacheConfig{
		Dir:     dir,
		MaxSize: 1 << 20,
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()

	// The shard ReadDir error should be handled gracefully.
	stats := dc.Stats()
	if stats.EntryCount != 0 {
		t.Fatalf("expected 0 entries, got %d", stats.EntryCount)
	}
}

func TestDataCache_EvictBackNil(t *testing.T) {
	// Test the back == nil break in evict when currentSize > MaxSize
	// but the list is empty. This is an edge case that shouldn't normally happen
	// but the code guards against it.
	dc := tempDataCache(t, 100)

	// Manually set currentSize > MaxSize without any entries.
	dc.mu.Lock()
	dc.currentSize = 200
	dc.mu.Unlock()

	dc.evict()

	// Should not panic; currentSize stays as-is since there's nothing to evict.
	dc.mu.RLock()
	size := dc.currentSize
	count := dc.order.Len()
	dc.mu.RUnlock()

	if count != 0 {
		t.Fatalf("expected 0 entries, got %d", count)
	}
	// currentSize remains 200 since there were no entries to evict.
	if size != 200 {
		t.Fatalf("expected currentSize 200, got %d", size)
	}
}

func TestNewDataCache_EvictionIntervalDefault(t *testing.T) {
	dir := t.TempDir()
	dc, err := NewDataCache(DataCacheConfig{
		Dir:              dir,
		MaxSize:          1 << 20,
		EvictionInterval: -1, // should become default 30s
	})
	if err != nil {
		t.Fatalf("NewDataCache: %v", err)
	}
	defer dc.Stop()
	if dc.config.EvictionInterval != 30*time.Second {
		t.Errorf("EvictionInterval = %v, want 30s", dc.config.EvictionInterval)
	}
}

func TestDataCacheTTLExpiry(t *testing.T) {
	dir := t.TempDir()
	dc, err := NewDataCache(DataCacheConfig{
		Dir:              dir,
		MaxSize:          10 * 1024 * 1024,
		EvictionInterval: time.Hour,
		DataTTL:          50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer dc.Stop()

	err = dc.Put("key1", "etag1", strings.NewReader("data"), 4)
	if err != nil {
		t.Fatal(err)
	}

	// Should hit immediately
	rc, ok := dc.Get("key1", "etag1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	rc.Close()

	// Wait for TTL
	time.Sleep(100 * time.Millisecond)

	// Should miss after TTL
	rc, ok = dc.Get("key1", "etag1")
	if ok {
		rc.Close()
		t.Fatal("expected cache miss after TTL")
	}
}
