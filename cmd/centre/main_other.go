// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux && !linux
// +build !linux,!linux

package main

import (
	"fmt"
	"net"
	"runtime"
	"sync"
)

// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

func dhcpServe(_ string, _ []net.IP, _ sync.WaitGroup) error {
	return fmt.Errorf("no DHCP service on %s", runtime.GOOS)
}
