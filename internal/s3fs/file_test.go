package s3fs

import (
	"io"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Tests: s3File (read-only file)
// ---------------------------------------------------------------------------

func TestS3FileRead(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	content := []byte("hello world, this is test data")
	mock.put("readable.txt", content, nil)

	f, err := fsys.Open("/readable.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 100)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != string(content) {
		t.Errorf("expected %q, got %q", content, buf[:n])
	}
}

func TestS3FileReadDirectory(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("dir/child.txt", []byte("x"), nil)

	f, err := fsys.Open("/dir")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 10)
	_, err = f.Read(buf)
	if err == nil {
		t.Fatal("expected error reading directory")
	}
}

func TestS3FileReadAfterClose(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("closeme.txt", []byte("data"), nil)

	f, err := fsys.Open("/closeme.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Read to initialize the chunk reader.
	buf := make([]byte, 10)
	f.Read(buf)

	f.Close()

	_, err = f.Read(buf)
	if err != os.ErrClosed {
		t.Fatalf("expected os.ErrClosed, got %v", err)
	}
}

func TestS3FileSeekStart(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("seekable.txt", []byte("abcdefghij"), nil)

	f, err := fsys.Open("/seekable.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	pos, err := f.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if pos != 5 {
		t.Errorf("expected pos 5, got %d", pos)
	}

	buf := make([]byte, 5)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read after seek: %v", err)
	}
	if string(buf[:n]) != "fghij" {
		t.Errorf("expected fghij, got %s", string(buf[:n]))
	}
}

func TestS3FileSeekCurrent(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("seekcur.txt", []byte("0123456789"), nil)

	f, err := fsys.Open("/seekcur.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Read 3 bytes to advance offset.
	buf := make([]byte, 3)
	f.Read(buf)

	// Seek +2 from current (offset 3 + 2 = 5).
	pos, err := f.Seek(2, io.SeekCurrent)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if pos != 5 {
		t.Errorf("expected pos 5, got %d", pos)
	}

	buf2 := make([]byte, 5)
	n, err := f.Read(buf2)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	if string(buf2[:n]) != "56789" {
		t.Errorf("expected 56789, got %s", string(buf2[:n]))
	}
}

func TestS3FileSeekEnd(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("seekend.txt", []byte("0123456789"), nil)

	f, err := fsys.Open("/seekend.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Seek to 3 bytes before end.
	pos, err := f.Seek(-3, io.SeekEnd)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if pos != 7 {
		t.Errorf("expected pos 7, got %d", pos)
	}

	buf := make([]byte, 5)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != "789" {
		t.Errorf("expected 789, got %s", string(buf[:n]))
	}
}

func TestS3FileSeekNegativeResult(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("seekneg.txt", []byte("data"), nil)

	f, err := fsys.Open("/seekneg.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	_, err = f.Seek(-100, io.SeekStart)
	if err == nil {
		t.Fatal("expected error for negative seek result")
	}
}

func TestS3FileSeekInvalidWhence(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("seekbad.txt", []byte("data"), nil)

	f, err := fsys.Open("/seekbad.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	_, err = f.Seek(0, 99)
	if err == nil {
		t.Fatal("expected error for invalid whence")
	}
}

func TestS3FileReaddir(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("parent/a.txt", []byte("aaa"), nil)
	mock.put("parent/b.txt", []byte("bb"), nil)
	mock.put("parent/sub/c.txt", []byte("c"), nil)

	f, err := fsys.Open("/parent")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	entries, err := f.Readdir(0)
	if err != nil {
		t.Fatalf("Readdir: %v", err)
	}

	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}

	// Check we got files and subdirectory.
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}

	if !names["a.txt"] {
		t.Error("expected a.txt in readdir results")
	}
	if !names["b.txt"] {
		t.Error("expected b.txt in readdir results")
	}
	if !names["sub"] {
		t.Error("expected sub directory in readdir results")
	}
}

func TestS3FileReaddirWithCount(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("countdir/a.txt", []byte("a"), nil)
	mock.put("countdir/b.txt", []byte("b"), nil)
	mock.put("countdir/c.txt", []byte("c"), nil)

	f, err := fsys.Open("/countdir")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	entries, err := f.Readdir(1)
	if err != nil {
		t.Fatalf("Readdir: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry with count=1, got %d", len(entries))
	}
}

func TestS3FileReaddirNotDir(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("file.txt", []byte("data"), nil)

	f, err := fsys.Open("/file.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	_, err = f.Readdir(0)
	if err == nil {
		t.Fatal("expected error for readdir on file")
	}
}

func TestS3FileReaddirCached(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("cachedir/x.txt", []byte("x"), nil)

	// First readdir populates cache.
	f1, err := fsys.Open("/cachedir")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entries1, err := f1.Readdir(0)
	f1.Close()
	if err != nil {
		t.Fatalf("first Readdir: %v", err)
	}

	// Second readdir should hit cache.
	f2, err := fsys.Open("/cachedir")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entries2, err := f2.Readdir(0)
	f2.Close()
	if err != nil {
		t.Fatalf("second Readdir: %v", err)
	}

	if len(entries1) != len(entries2) {
		t.Errorf("cached readdir returned different count: %d vs %d", len(entries1), len(entries2))
	}
}

func TestS3FileClose(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("closeable.txt", []byte("data"), nil)

	f, err := fsys.Open("/closeable.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// First close.
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Second close should be no-op.
	if err := f.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestS3FileCloseWithChunkedReader(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("chunked.txt", []byte("some data"), nil)

	f, err := fsys.Open("/chunked.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Read to initialize chunk reader.
	buf := make([]byte, 4)
	f.Read(buf)

	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestS3FileWrite(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("readonly.txt", []byte("data"), nil)

	f, err := fsys.Open("/readonly.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	_, err = f.Write([]byte("new"))
	if err == nil {
		t.Fatal("expected error writing to read-only file")
	}
}

func TestS3FileTruncate(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("trunc.txt", []byte("data"), nil)

	f, err := fsys.Open("/trunc.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	s3f := f.(*s3File)
	err = s3f.Truncate()
	if err == nil {
		t.Fatal("expected error for truncate on read-only file")
	}
}

func TestS3FileSync(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("syncme.txt", []byte("data"), nil)

	f, err := fsys.Open("/syncme.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	s3f := f.(*s3File)
	err = s3f.Sync()
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

func TestS3FileStat(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("stat.txt", []byte("hello"), nil)

	f, err := fsys.Open("/stat.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size() != 5 {
		t.Errorf("expected size 5, got %d", fi.Size())
	}
}

func TestS3FileName(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("named.txt", []byte("x"), nil)

	f, err := fsys.Open("/named.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	if f.Name() != "/named.txt" {
		t.Errorf("expected /named.txt, got %s", f.Name())
	}
}

// ---------------------------------------------------------------------------
// Tests: s3WritableFile
// ---------------------------------------------------------------------------

func TestWritableFileWriteAndClose(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/upload.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	data := []byte("uploaded content")
	n, err := f.Write(data)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected write %d bytes, got %d", len(data), n)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify data was uploaded to mock S3.
	mock.mu.RLock()
	obj, ok := mock.objects["upload.txt"]
	mock.mu.RUnlock()
	if !ok {
		t.Fatal("expected upload.txt in S3 after close")
	}
	if string(obj.data) != "uploaded content" {
		t.Errorf("expected 'uploaded content', got %q", string(obj.data))
	}
}

func TestWritableFileDoubleClose(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/dblclose.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Second close should be no-op.
	if err := f.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestWritableFileWriteAfterClose(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/wac.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	f.Close()

	_, err = f.Write([]byte("late"))
	if err != os.ErrClosed {
		t.Fatalf("expected os.ErrClosed, got %v", err)
	}
}

func TestWritableFileTruncate(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/truncw.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	f.Write([]byte("some data"))

	wf := f.(*s3WritableFile)
	if err := wf.Truncate(); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
}

func TestWritableFileTruncateAfterClose(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/trunc2w.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	f.Close()

	wf := f.(*s3WritableFile)
	err = wf.Truncate()
	if err != os.ErrClosed {
		t.Fatalf("expected os.ErrClosed, got %v", err)
	}
}

func TestWritableFileSeek(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/seekw.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	f.Write([]byte("hello"))

	pos, err := f.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if pos != 0 {
		t.Errorf("expected pos 0, got %d", pos)
	}
}

func TestWritableFileSeekAfterClose(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/seekwc.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	f.Close()

	_, err = f.Seek(0, io.SeekStart)
	if err != os.ErrClosed {
		t.Fatalf("expected os.ErrClosed, got %v", err)
	}
}

func TestWritableFileRead(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/readw.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 10)
	_, err = f.Read(buf)
	if err == nil {
		t.Fatal("expected error reading write-only file")
	}
}

func TestWritableFileReaddir(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/readdirw.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	_, err = f.Readdir(0)
	if err == nil {
		t.Fatal("expected error for readdir on writable file")
	}
}

func TestWritableFileStat(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/statw.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Name() != "statw.txt" {
		t.Errorf("expected statw.txt, got %s", fi.Name())
	}
}

func TestWritableFileName(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/named-w.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	if f.Name() != "/named-w.txt" {
		t.Errorf("expected /named-w.txt, got %s", f.Name())
	}
}

func TestWritableFileSync(t *testing.T) {
	fsys, _, cleanup := setupTestFS(t)
	defer cleanup()

	f, err := fsys.OpenFile("/syncw.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	wf := f.(*s3WritableFile)
	if err := wf.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: Readdir with no cache
// ---------------------------------------------------------------------------

func TestReaddirNoCache(t *testing.T) {
	fsys, mock, cleanup := setupTestFSNoCache(t)
	defer cleanup()

	mock.put("ncdir/f1.txt", []byte("a"), nil)
	mock.put("ncdir/f2.txt", []byte("b"), nil)

	f, err := fsys.Open("/ncdir")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	entries, err := f.Readdir(0)
	if err != nil {
		t.Fatalf("Readdir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// Tests: Seek on s3File with pre-initialized chunked reader
// ---------------------------------------------------------------------------

func TestS3FileSeekWithChunkedReader(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("seekchunk.txt", []byte("abcdefghijklmnop"), nil)

	f, err := fsys.Open("/seekchunk.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Read some bytes to initialize chunked reader.
	buf := make([]byte, 4)
	f.Read(buf)

	// Now seek -- this should go through the chunked reader's Seek path.
	pos, err := f.Seek(8, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if pos != 8 {
		t.Errorf("expected pos 8, got %d", pos)
	}

	buf2 := make([]byte, 4)
	n, err := f.Read(buf2)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	if string(buf2[:n]) != "ijkl" {
		t.Errorf("expected ijkl, got %s", string(buf2[:n]))
	}
}

// ---------------------------------------------------------------------------
// Tests: Seek on s3File at same position (no-op path)
// ---------------------------------------------------------------------------

func TestS3FileSeekSamePosition(t *testing.T) {
	fsys, mock, cleanup := setupTestFS(t)
	defer cleanup()

	mock.put("seeknoop.txt", []byte("abcdefgh"), nil)

	f, err := fsys.Open("/seeknoop.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// Read to initialize chunked reader and advance offset.
	buf := make([]byte, 4)
	f.Read(buf)

	// Seek to same position (offset 4) -- should be a no-op for chunk reader.
	pos, err := f.Seek(4, io.SeekStart)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if pos != 4 {
		t.Errorf("expected pos 4, got %d", pos)
	}
}
