// Copyright 2018 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// centre is used to support one or more of DHCP, TFTP, and HTTP services
// on harvey networks.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"harvey-os.org/ninep/protocol"
	"harvey-os.org/ninep/ufs"
	"pack.ag/tftp"
)

var (
	// TODO: get info from centre ipv4
	inf        = flag.String("i", "eth0", "Interface to serve DHCPv4 on")
	dnsServers = flag.String("dns", "", "Comma-separated list of DNS servers for DHCPv4")

	// File serving
	tftpDir    = flag.String("tftp-dir", "", "Directory to serve over TFTP")
	tftpPort   = flag.Int("tftp-port", 69, "Port to serve TFTP on")
	httpDir    = flag.String("http-dir", "", "Directory to serve over HTTP")
	httpPort   = flag.Int("http-port", 80, "Port to serve HTTP on")
	ninepDir   = flag.String("ninep-dir", "", "Directory to serve over 9p")
	ninepAddr  = flag.String("ninep-addr", ":5640", "addr to serve 9p on")
	ninepDebug = flag.Int("ninep-debug", 0, "Debug level for ninep -- for now, only non-zero matters")
)

// lookupIP looks up an IP address corresponding to the given name.
// It also returns a slice of other hostnames it found for the same IP.
func lookupIP(hostFile string, addr string) (net.IP, []string, error) {
	var err error
	// First try the override
	if hostFile != `` {
		// Read the file and walk each line looking for a match.
		// We do this so you can update the file without restarting the server
		var f *os.File
		if f, err = os.Open(hostFile); err != nil {
			return nil, nil, err
		}
		defer f.Close()
		// We're going to be real simple-minded. We take each line to consist of
		// one IP, followed by one or more whitespace-separated hostnames, with
		// the mac address at the end:
		// <ip> <hostname>... <mac>
		scan := bufio.NewScanner(f)
		for scan.Scan() {
			fields := strings.Fields(scan.Text())
			if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
				continue
			}
			var hostnames []string
			for _, fld := range fields[1:] {
				if strings.ToLower(fld) == strings.ToLower(addr) {
					return net.ParseIP(fields[0]), hostnames, nil
				}
				hostnames = append(hostnames, fld)
			}
		}
	}
	// Now just do a regular lookup since we didn't find it in the override
	ips, err := net.LookupIP(addr)
	if err != nil {
		return nil, nil, err
	}
	if len(ips) == 0 {
		return nil, nil, errors.New("No IP found")
	}
	ip := ips[0]
	names, err := net.LookupAddr(ip.String())
	return ip, names, err
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

	if *inf != "" {
		if err := dhcpServe(*inf, dns, wg); err != nil {
			log.Fatal(err)
		}
	}
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
