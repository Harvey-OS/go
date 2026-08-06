// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// raspiModifiers is unimplemented on darwin: the upstream bsdp package pulls
// in a client4 raw-socket dependency that only builds on linux.
func raspiModifiers() []dhcpv4.Modifier {
	log.Printf("-raspi is not supported on darwin; ignoring")
	return nil
}
