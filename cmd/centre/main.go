// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// centre is used to support one or more of DHCP, TFTP, and HTTP services
// on harvey networks.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	"harvey-os.org/ninep/protocol"
	"harvey-os.org/ninep/ufs"
	"pack.ag/tftp"
)

var (
	// TODO: get info from centre ipv4
	inf = flag.String("i", "eth0", "Interface to serve DHCPv4 on")

	// DHCPv4-specific
	ipv4         = flag.Bool("4", true, "IPv4 DHCP server")
	rootpath     = flag.String("rootpath", "", "RootPath option to serve via DHCPv4")
	bootfilename = flag.String("bootfilename", "pxelinux.0", "Boot file to serve via DHCPv4")
	raspi        = flag.Bool("raspi", false, "Configure to boot Raspberry Pi")
	dnsServers   = flag.String("dns", "", "Comma-separated list of DNS servers for DHCPv4")
	gateway      = flag.String("gw", "", "Optional gateway IP for DHCPv4")

	// DHCPv6-specific
	ipv6           = flag.Bool("6", false, "DHCPv6 server")
	v6Bootfilename = flag.String("v6-bootfilename", "", "Boot file to serve via DHCPv6")

	// File serving
	tftpDir    = flag.String("tftp-dir", "", "Directory to serve over TFTP")
	tftpPort   = flag.Int("tftp-port", 69, "Port to serve TFTP on")
	httpDir    = flag.String("http-dir", "", "Directory to serve over HTTP")
	httpPort   = flag.Int("http-port", 80, "Port to serve HTTP on")
	ninepDir   = flag.String("ninep-dir", "", "Directory to serve over 9p")
	ninepAddr  = flag.String("ninep-addr", ":5640", "addr to serve 9p on")
	ninepDebug = flag.Int("ninep-debug", 0, "Debug level for ninep -- for now, only non-zero matters")
)

type dserver4 struct {
	mac          net.HardwareAddr
	yourIP       net.IP
	submask      net.IPMask
	self         net.IP
	bootfilename string
	rootpath     string
	dns          []net.IP
}

func main() {
	flag.Parse()

	var wg sync.WaitGroup
	if len(*tftpDir) != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			server, err := tftp.NewServer(fmt.Sprintf(":%d", *tftpPort))
			if err != nil {
				log.Fatalf("Could not start TFTP server: %v", err)
			}

			log.Println("starting file server")
			server.ReadHandler(tftp.FileServer(*tftpDir))
			log.Fatal(server.ListenAndServe())
		}()
	}
	if len(*httpDir) != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			http.Handle("/", http.FileServer(http.Dir(*httpDir)))
			log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *httpPort), nil))
		}()
	}

	var dns []net.IP
	parts := strings.Split(*dnsServers, ",")
	for _, p := range parts {
		ip := net.ParseIP(p)
		if ip != nil {
			dns = append(dns, ip)
		}
	}

	// dhcp is a catch-all function for everything, so we can make it an optional
	// component on non-linux systems.
	dhcp()
	// TODO: serve on ip6
	if len(*ninepDir) != 0 {
		ln, err := net.Listen("tcp4", *ninepAddr)
		if err != nil {
			log.Fatalf("Listen failed: %v", err)
		}

		ufslistener, err := ufs.NewUFS(*ninepDir, *ninepDebug, func(l *protocol.NetListener) error {
			l.Trace = nil
			if *ninepDebug > 1 {
				l.Trace = log.Printf
			}
			return nil
		})

		if err != nil {
			log.Fatal(err)
		}
		if err := ufslistener.Serve(ln); err != nil {
			log.Fatal(err)
		}

	}
	wg.Wait()
}
