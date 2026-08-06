// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"testing"

	"harvey-os.org/ninep/protocol"
	"harvey-os.org/ninep/tmpfs"
)

func newTestFileServer(t *testing.T) (*fileServer, protocol.FID) {
	t.Helper()
	arch, err := tmpfs.ReadImageTar(createTestImage(t, 0))
	if err != nil {
		t.Fatal(err)
	}
	fs := &fileServer{archive: arch, files: make(map[protocol.FID]*FidEntry), ioUnit: 1 * 1024 * 1024}
	root := protocol.FID(1)
	if _, err := fs.Rattach(root, protocol.NOFID, "test", ""); err != nil {
		t.Fatal(err)
	}
	return fs, root
}

func walkTo(t *testing.T, fs *fileServer, fid, newfid protocol.FID, path ...string) {
	t.Helper()
	if _, err := fs.Rwalk(fid, newfid, path); err != nil {
		t.Fatalf("Rwalk(%v): %v", path, err)
	}
}

// Files and directories in tmpfs are never marked executable (see
// tmpfs.File.P9Dir and tmpfs.Directory.P9Dir), so Ropen with OEXEC must
// always be denied.
func TestRopenExecPermission(t *testing.T) {
	fs, root := newTestFileServer(t)

	fileFid := protocol.FID(2)
	walkTo(t, fs, root, fileFid, "readme.txt")

	if _, _, err := fs.Ropen(fileFid, protocol.OREAD); err != nil {
		t.Errorf("Ropen(OREAD) on a file: got %v, want nil", err)
	}
	if _, _, err := fs.Ropen(fileFid, protocol.OEXEC); err == nil {
		t.Errorf("Ropen(OEXEC) on a non-executable file: got nil error, want permission denied")
	}

	dirFid := protocol.FID(3)
	walkTo(t, fs, root, dirFid, "foo")
	if _, _, err := fs.Ropen(dirFid, protocol.OEXEC); err == nil {
		t.Errorf("Ropen(OEXEC) on a directory: got nil error, want permission denied")
	}
}

// Rread on a directory must be able to return more than one entry per call
// when they fit in count; a prior bug always returned after the first one.
func TestRreadMultipleDirEntries(t *testing.T) {
	fs, root := newTestFileServer(t)

	dirFid := protocol.FID(2)
	walkTo(t, fs, root, dirFid, "foo")

	b, err := fs.Rread(dirFid, 0, 4096)
	if err != nil {
		t.Fatal(err)
	}

	buf := bytes.NewBuffer(b)
	n := 0
	for buf.Len() > 0 {
		if _, err := protocol.Unmarshaldir(buf); err != nil {
			t.Fatalf("Unmarshaldir: %v", err)
		}
		n++
	}
	if n < 2 {
		t.Errorf("Rread returned %d directory entries in one call, want at least 2", n)
	}
}
