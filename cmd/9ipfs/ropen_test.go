// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"harvey-os.org/cmd/9ipfs/internal/ipfs"
	"harvey-os.org/ninep/protocol"
)

// Every entry served by 9ipfs is reported as mode 0555 (see plan9Dir), so
// exec opens must always succeed for both files and directories, while
// write opens must always be denied since this is a read-only filesystem.
func TestRopenExec(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repoDir, err := ioutil.TempDir("", "9ipfs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(repoDir)

	ipfsNode, err := ipfs.New(ctx, &ipfs.Config{
		LogWriter:   os.Stderr,
		TempRepo:    true,
		TempRepoDir: repoDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	testdata, err := ipfsNode.Put(ctx, "testdata")
	if err != nil {
		t.Fatal(err)
	}

	fs := newFileServer(ipfsNode)

	root := protocol.FID(0)
	if _, err := fs.Rattach(root, protocol.NOFID, "test", ""); err != nil {
		t.Fatal(err)
	}

	dirFid := protocol.FID(1)
	if _, err := fs.Rwalk(root, dirFid, []string{testdata.Cid.String()}); err != nil {
		t.Fatalf("Rwalk to testdata dir: %v", err)
	}
	if _, _, err := fs.Ropen(dirFid, protocol.OEXEC); err != nil {
		t.Errorf("Ropen(dir, OEXEC) = %v, want nil", err)
	}

	fileFid := protocol.FID(2)
	if _, err := fs.Rwalk(dirFid, fileFid, []string{"hello.txt"}); err != nil {
		t.Fatalf("Rwalk to hello.txt: %v", err)
	}
	if _, _, err := fs.Ropen(fileFid, protocol.OEXEC); err != nil {
		t.Errorf("Ropen(file, OEXEC) = %v, want nil", err)
	}
	if _, _, err := fs.Ropen(fileFid, protocol.OWRITE); err == nil {
		t.Errorf("Ropen(file, OWRITE) = nil, want error")
	}
}
