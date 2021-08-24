// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/bsdp"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/server6"
)

var (
	// DHCPv4-specific
	ipv4         = flag.Bool("4", true, "IPv4 DHCP server")
	selfIP       = flag.String("ip", "192.168.0.1", "DHCPv4 IP of self")
	rootpath     = flag.String("rootpath", "", "RootPath option to serve via DHCPv4")
	bootfilename = flag.String("bootfilename", "pxelinux.0", "Boot file to serve via DHCPv4")
	raspi        = flag.Bool("raspi", false, "Configure to boot Raspberry Pi")
	gateway      = flag.String("gw", "", "Optional gateway IP for DHCPv4")
	hostFile     = flag.String("hostfile", "", "Optional additional hosts file for DHCPv4")

	// DHCPv6-specific
	ipv6           = flag.Bool("6", false, "DHCPv6 server")
	v6Bootfilename = flag.String("v6-bootfilename", "", "Boot file to serve via DHCPv6")
)

type dserver4 struct {
	mac          net.HardwareAddr
	yourIP       net.IP
	submask      net.IPMask
	self         net.IP
	bootfilename string
	rootpath     string
	dns          []net.IP
	hostFile     string
}

func (s *dserver4) dhcpHandler(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {
	log.Printf("Handling request %v for peer %v", m, peer)

	var replyType dhcpv4.MessageType
	switch mt := m.MessageType(); mt {
	case dhcpv4.MessageTypeDiscover:
		replyType = dhcpv4.MessageTypeOffer
	case dhcpv4.MessageTypeRequest:
		replyType = dhcpv4.MessageTypeAck
	default:
		log.Printf("Can't handle type %v", mt)
		return
	}

	ip, hostnames, err := lookupIP(s.hostFile, fmt.Sprintf("u%s", m.ClientHWAddr))
	if err != nil || ip.IsUnspecified() {
		log.Printf("Not responding to DHCP request for mac %s", m.ClientHWAddr)
		log.Printf("You can create a host entry of the form 'a.b.c.d [names] u%s' 'ip6addr [names] u%s'if you wish", m.ClientHWAddr, m.ClientHWAddr)
		return
	}

	// Since this is dserver4, we force it to be an ip4 address.
	ip = ip.To4()

	// We're just going to use the first hostname for now
	var hostname string
	if len(hostnames) > 0 {
		hostname = hostnames[0]
	}

	modifiers := []dhcpv4.Modifier{
		dhcpv4.WithMessageType(replyType),
		dhcpv4.WithServerIP(s.self),
		dhcpv4.WithRouter(s.self),
		dhcpv4.WithNetmask(s.submask),
		dhcpv4.WithYourIP(ip),
		// RFC 2131, Section 4.3.1. Server Identifier: MUST
		dhcpv4.WithOption(dhcpv4.OptServerIdentifier(s.self)),
		// RFC 2131, Section 4.3.1. IP lease time: MUST
		dhcpv4.WithOption(dhcpv4.OptIPAddressLeaseTime(dhcpv4.MaxLeaseTime)),
	}
	if hostname != `` {
		modifiers = append(modifiers, dhcpv4.WithOption(dhcpv4.OptHostName(hostname)))
	}
	if *raspi {
		modifiers = append(modifiers,
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
		)
	}
	if len(s.dns) != 0 {
		modifiers = append(modifiers, dhcpv4.WithDNS(s.dns...))
	}
	if *gateway != `` {
		modifiers = append(modifiers, dhcpv4.WithGatewayIP(net.ParseIP(*gateway)))
		modifiers = append(modifiers, dhcpv4.WithRouter(net.ParseIP(*gateway)))
	}
	reply, err := dhcpv4.NewReplyFromRequest(m, modifiers...)

	// RFC 6842, MUST include Client Identifier if client specified one.
	if val := m.Options.Get(dhcpv4.OptionClientIdentifier); len(val) > 0 {
		reply.UpdateOption(dhcpv4.OptGeneric(dhcpv4.OptionClientIdentifier, val))
	}
	if len(s.bootfilename) > 0 {
		reply.BootFileName = s.bootfilename
	}
	if len(s.rootpath) > 0 {
		reply.UpdateOption(dhcpv4.OptRootPath(s.rootpath))
	}
	if err != nil {
		log.Printf("Could not create reply for %v: %v", m, err)
		return
	}

	// Experimentally determined. You can't just blindly send a broadcast packet
	// with the broadcast address. You can, however, send a broadcast packet
	// to a subnet for an interface. That actually makes some sense.
	// This fixes the observed problem that OSX just swallows these
	// packets if the peer is 255.255.255.255.
	// I chose this way of doing it instead of files with build constraints
	// because this is not that expensive and it's just a tiny bit easier to
	// follow IMHO.
	if runtime.GOOS == "darwin" {
		p := &net.UDPAddr{IP: s.yourIP.Mask(s.submask), Port: 68}
		log.Printf("Changing %v to %v", peer, p)
		peer = p
	}

	log.Printf("Sending %v to %v", reply.Summary(), peer)
	if _, err := conn.WriteTo(reply.ToBytes(), peer); err != nil {
		log.Printf("Could not write %v: %v", reply, err)
	}
}

type dserver6 struct {
	mac         net.HardwareAddr
	yourIP      net.IP
	bootfileurl string
}

func (s *dserver6) dhcpHandler(conn net.PacketConn, peer net.Addr, m dhcpv6.DHCPv6) {
	log.Printf("Handling DHCPv6 request %v sent by %v", m.Summary(), peer.String())

	msg, err := m.GetInnerMessage()
	if err != nil {
		log.Printf("Could not find unpacked message: %v", err)
		return
	}

	if msg.MessageType != dhcpv6.MessageTypeSolicit {
		log.Printf("Only accept SOLICIT message type, this is a %s", msg.MessageType)
		return
	}
	if msg.GetOneOption(dhcpv6.OptionRapidCommit) == nil {
		log.Printf("Only accept requests with rapid commit option.")
		return
	}
	if mac, err := dhcpv6.ExtractMAC(msg); err != nil {
		log.Printf("No MAC address in request: %v", err)
		return
	} else if s.mac != nil && !bytes.Equal(s.mac, mac) {
		log.Printf("MAC address %s doesn't match expected MAC %s", mac, s.mac)
		return
	}

	// From RFC 3315, section 17.1.4, If the client includes a Rapid Commit
	// option in the Solicit message, it will expect a Reply message that
	// includes a Rapid Commit option in response.
	reply, err := dhcpv6.NewReplyFromMessage(msg)
	if err != nil {
		log.Printf("Failed to create reply for %v: %v", m, err)
		return
	}

	iana := msg.Options.OneIANA()
	if iana != nil {
		iana.Options.Update(&dhcpv6.OptIAAddress{
			IPv6Addr:          s.yourIP,
			PreferredLifetime: math.MaxUint32 * time.Second,
			ValidLifetime:     math.MaxUint32 * time.Second,
		})
		reply.AddOption(iana)
	}
	if len(s.bootfileurl) > 0 {
		reply.Options.Add(dhcpv6.OptBootFileURL(s.bootfileurl))
	}

	if _, err := conn.WriteTo(reply.ToBytes(), peer); err != nil {
		log.Printf("Failed to send response %v: %v", reply, err)
		return
	}

	log.Printf("DHCPv6 request successfully handled, reply: %v", reply.Summary())
}

func dhcpServe(inf string, dns []net.IP, wg sync.WaitGroup) error {
	centre, _, err := lookupIP(*hostFile, "centre")
	var ip net.IP
	if err != nil {
		log.Printf("No centre entry found via LookupIP: not serving DHCP")
	}
	if *ipv4 {
		if err == nil {
			ip = centre.To4()
		} else {
			ip = net.ParseIP(*selfIP)
		}
		wg.Add(1)
		log.Printf("Using IP address %v on %v", ip, inf)
		go func() {
			defer wg.Done()
			s := &dserver4{
				self:         ip,
				bootfilename: *bootfilename,
				rootpath:     *rootpath,
				submask:      ip.DefaultMask(),
				dns:          dns,
				hostFile:     *hostFile,
			}

			laddr := &net.UDPAddr{Port: dhcpv4.ServerPort}
			server, err := server4.NewServer(inf, laddr, s.dhcpHandler)
			if err != nil {
				log.Fatal(err)
			}
			if err := server.Serve(); err != nil {
				log.Fatal(err)
			}
		}()
	}

	// not yet.
	if false && *ipv6 && inf != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()

			s := &dserver6{
				bootfileurl: *v6Bootfilename,
			}
			laddr := &net.UDPAddr{
				IP:   net.IPv6unspecified,
				Port: dhcpv6.DefaultServerPort,
			}
			server, err := server6.NewServer("eth0", laddr, s.dhcpHandler)
			if err != nil {
				log.Fatal(err)
			}

			log.Println("starting dhcpv6 server")
			if err := server.Serve(); err != nil {
				log.Fatal(err)
			}
		}()
	}
	return nil
}
