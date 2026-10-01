//go:build wasm

package tests

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"webtyp.com/files"
	"webtyp.com/opfs"
)

// TestLargeFileCrossesChunks tests writing and reading back a 20 MiB file across chunks, then appending 1 MiB.
func TestLargeFileCrossesChunks(t *testing.T) {
	fs, err := opfs.Open(fmt.Sprintf("test-large/%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}

	size20 := 20 * 1024 * 1024
	data20 := make([]byte, size20)
	for i := 0; i < size20; i++ {
		data20[i] = byte(i % 251)
	}

	if err := fs.WriteFile("large.bin", data20); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	read20, err := fs.ReadFile("large.bin")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(read20, data20) {
		t.Fatal("ReadFile returned data differing from data20")
	}

	size1 := 1 * 1024 * 1024
	data1 := make([]byte, size1)
	for i := 0; i < size1; i++ {
		data1[i] = byte((i + 17) % 251)
	}

	if err := fs.AppendFile("large.bin", data1); err != nil {
		t.Fatalf("AppendFile failed: %v", err)
	}

	read21, err := fs.ReadFile("large.bin")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(read21) != size20+size1 {
		t.Fatalf("expected length %d, got %d", size20+size1, len(read21))
	}
	if !bytes.Equal(read21[:size20], data20) {
		t.Fatal("prefix does not match data20")
	}
	if !bytes.Equal(read21[size20:], data1) {
		t.Fatal("appended bytes do not match data1")
	}
}

// TestNestedPaths tests nested paths, missing files, and missing directories returning files.ErrNotExist.
func TestNestedPaths(t *testing.T) {
	fs, err := opfs.Open(fmt.Sprintf("test-nested/%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("hello nested")
	if err := fs.WriteFile("a/b/c.bin", content); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := fs.ReadFile("a/b/c.bin")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("expected %q, got %q", content, got)
	}

	_, err = fs.ReadFile("a/x/y.bin")
	if err != files.ErrNotExist {
		t.Fatalf("expected files.ErrNotExist, got %v", err)
	}

	_, err = fs.ReadFile("a/b/none.bin")
	if err != files.ErrNotExist {
		t.Fatalf("expected files.ErrNotExist, got %v", err)
	}
}

// TestInvalidPaths tests invalid relative paths returning invalid path error text.
func TestInvalidPaths(t *testing.T) {
	fs, err := opfs.Open(fmt.Sprintf("test-invalid/%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}

	invalidPaths := []string{"", "/a", "a/", "a//b", "./a", "a/../b"}
	for _, path := range invalidPaths {
		expectedErrStr := fmt.Sprintf("opfs: invalid path %q", path)

		_, err := fs.ReadFile(path)
		if err == nil || err.Error() != expectedErrStr {
			t.Errorf("ReadFile(%q): expected %q, got %v", path, expectedErrStr, err)
		}

		err = fs.WriteFile(path, []byte("x"))
		if err == nil || err.Error() != expectedErrStr {
			t.Errorf("WriteFile(%q): expected %q, got %v", path, expectedErrStr, err)
		}
	}
}

// TestOpenSameDirectoryTwice tests reading a file from a second FS handle opened on the same directory.
func TestOpenSameDirectoryTwice(t *testing.T) {
	dirName := fmt.Sprintf("test-shared/%d", time.Now().UnixNano())

	fs1, err := opfs.Open(dirName)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("shared content")
	if err := fs1.WriteFile("item.txt", data); err != nil {
		t.Fatalf("fs1 WriteFile failed: %v", err)
	}

	fs2, err := opfs.Open(dirName)
	if err != nil {
		t.Fatal(err)
	}

	got, err := fs2.ReadFile("item.txt")
	if err != nil {
		t.Fatalf("fs2 ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("expected %q, got %q", data, got)
	}
}

// TestRemoveFileNested removes a file in a subdirectory, keeps its sibling, and refuses to
// remove the directory itself.
func TestRemoveFileNested(t *testing.T) {
	fs, err := opfs.Open(fmt.Sprintf("test-remove/%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile("cote/v1/model.bin", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile("cote/v1/keep.bin", []byte("keep")); err != nil {
		t.Fatal(err)
	}
	if err := fs.RemoveFile("cote/v1/model.bin"); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if _, err := fs.ReadFile("cote/v1/model.bin"); err != files.ErrNotExist {
		t.Fatalf("ReadFile(removed) = %v, want files.ErrNotExist", err)
	}
	if got, err := fs.ReadFile("cote/v1/keep.bin"); err != nil || string(got) != "keep" {
		t.Fatalf("sibling = %q, %v; want \"keep\"", got, err)
	}
	if err := fs.RemoveFile("cote/v1"); err == nil {
		t.Fatal("RemoveFile(directory) succeeded; it must only remove files")
	}
	if got, err := fs.ReadFile("cote/v1/keep.bin"); err != nil || string(got) != "keep" {
		t.Fatalf("after RemoveFile(directory) sibling = %q, %v; want \"keep\"", got, err)
	}
	if err := fs.RemoveFile("cote/missing/x.bin"); err != files.ErrNotExist {
		t.Fatalf("RemoveFile(missing dir) = %v, want files.ErrNotExist", err)
	}
}
