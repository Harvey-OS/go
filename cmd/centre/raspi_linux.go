// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/bsdp"
)

// raspiModifiers returns the DHCPv4 options needed for a Raspberry Pi to
// recognize the offer as a PXE native boot menu.
func raspiModifiers() []dhcpv4.Modifier {
	return []dhcpv4.Modifier{
		dhcpv4.WithOption(dhcpv4.OptClassIdentifier("PXEClient")),
		// Add option 43, suboption 9 (PXE native boot menu) to allow Raspberry Pi to recognise the offer
		dhcpv4.WithOption(dhcpv4.Option{
			Code: dhcpv4.OptionVendorSpecificInformation,
			Value: bsdp.VendorOptions{Options: dhcpv4.OptionsFromList(
				// The dhcp package only seems to support Apple BSDP boot menu items,
				// so we have to craft the option by hand.
				// \x11 is the length of the 'Raspberry Pi Boot' string...
				dhcpv4.OptGeneric(dhcpv4.GenericOptionCode(9), []byte("\000\000\x11Raspberry Pi Boot")),
			)},
		}),
	}
}
