// Copyright 2012 The Ninep Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// This code is imported from the old ninep repo,
// with some changes.

package protocol

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opentracing/opentracing-go"
)

const DefaultAddr = ":5640"

type NsCreator func() NineServer

// NetListener is a struct used to control how we listen for remote connections.
type NetListener struct {
	nsCreator NsCreator

	// TCP address to listen on, default is DefaultAddr
	Addr string

	// Trace function for logging
	Trace Tracer

	// mu guards below
	mu sync.Mutex

	listeners map[net.Listener]struct{}

	tracer opentracing.Tracer
}

// Server is a 9p server.
// For now it's extremely serial. But we will use a chan for replies to ensure that
// we can go to a more concurrent one later.
type Server struct {
	NS NineServer
	D  Dispatcher

	// Versioned is set to true on the first call to Tversion
	Versioned bool
}

// conn has a listener in it, and I don't recall why.
type conn struct {
	listener *NetListener

	// server on which the connection arrived.
	server *Server

	io.Reader
	io.Writer
	io.Closer

	// remoteAddr is rwc.RemoteAddr().String(). See note in net/http/server.go.
	remoteAddr string

	// replies
	replies chan RPCReply

	// dead is set to true when we finish reading packets.
	dead bool

	logger func(string, ...interface{})

	span opentracing.Span

	requestCounter uint64
}

// this is getting icky, but the plan is to deprecate this whole thing in favor of p9.
// So it's ok.
var Debug = func(string, ...interface{}) {}

// NewNetListener returns a NetListener, on which new sessions may be established.
func NewNetListener(nsCreator NsCreator, opts ...NetListenerOpt) (*NetListener, error) {
	l := &NetListener{
		nsCreator: nsCreator,
	}
	if opentracing.IsGlobalTracerRegistered() {
		l.tracer = opentracing.GlobalTracer()
	} else {
		l.tracer = opentracing.NoopTracer{}
	}
	for _, o := range opts {
		if err := o(l); err != nil {
			return nil, err
		}
	}

	return l, nil
}

func (l *NetListener) newConn(rwc net.Conn) (*conn, error) {
	ns := l.nsCreator()
	server := &Server{NS: ns, D: Dispatch}

	c := &conn{
		server:     server,
		listener:   l,
		Reader:     rwc,
		Writer:     rwc,
		Closer:     rwc,
		replies:    make(chan RPCReply, NumTags),
		remoteAddr: rwc.RemoteAddr().String(),
		logger:     l.logf,
		span:       l.tracer.StartSpan("newConn"),
	}

	return c, nil
}

// ServeFromRWC runs a server from an io.ReadWriteCloser
// This can be used on Plan 9 for files in #s (i.e. /srv)
func ServeFromRWC(rwc io.ReadWriteCloser, fs NineServer, n string) {
	var tracer opentracing.Tracer
	tracer = opentracing.NoopTracer{}
	if opentracing.IsGlobalTracerRegistered() {
		tracer = opentracing.GlobalTracer()
	}
	c := &conn{
		server:     &Server{NS: fs, D: Dispatch},
		Reader:     rwc,
		Writer:     rwc,
		Closer:     rwc,
		replies:    make(chan RPCReply, NumTags),
		remoteAddr: n,
		logger:     Debug,
		span:       tracer.StartSpan("ServeFromRWC"),
	}

	c.serve()
}

// trackNetListener from http.Server
func (l *NetListener) trackNetListener(ln net.Listener, add bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.listeners == nil {
		l.listeners = make(map[net.Listener]struct{})
	}

	if add {
		l.listeners[ln] = struct{}{}
	} else {
		delete(l.listeners, ln)
	}
}

// closeNetListenersLocked from http.Server
func (l *NetListener) closeNetListenersLocked() error {
	var err error
	for ln := range l.listeners {
		if cerr := ln.Close(); cerr != nil && err == nil {
			err = cerr
		}
		delete(l.listeners, ln)
	}
	return err
}

// Serve accepts incoming connections on the NetListener and calls e.Accept on
// each connection.
func (l *NetListener) Serve(ln net.Listener) error {
	defer ln.Close()

	var tempDelay time.Duration // how long to sleep on accept failure

	l.trackNetListener(ln, true)
	defer l.trackNetListener(ln, false)

	// from http.Server.Serve
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				l.logf("ufs: Accept error: %v; retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return err
		}
		tempDelay = 0

		if err := l.Accept(conn); err != nil {
			return err
		}
	}
}

// Accept a new connection, typically called via Serve but may be called
// directly if there's a connection from an exotic listener.
func (l *NetListener) Accept(conn net.Conn) error {
	c, err := l.newConn(conn)
	if err != nil {
		return err
	}

	go c.serve()
	return nil
}

// Shutdown closes all active listeners. It does not close all active
// connections but probably should.
func (l *NetListener) Shutdown() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.closeNetListenersLocked()
}

func (l *NetListener) String() string {
	// TODO
	return ""
}

func (l *NetListener) logf(format string, args ...interface{}) {
	if l.Trace != nil {
		l.Trace(format, args...)
	}
}

func (c *conn) String() string {
	return fmt.Sprintf("Dead %v %d replies pending", c.dead, len(c.replies))
}

func (c *conn) logf(format string, args ...interface{}) {
	// prepend some info about the conn
	if c.logger != nil {
		c.logger("[%v] "+format, append([]interface{}{c.remoteAddr}, args...)...)
	}
}

func (c *conn) serve() {
	ctx, cancel := context.WithCancel(context.Background())
	defer c.Close()
	defer cancel()
	c.logf("Starting readNetPackets")
	sp := c.span.Tracer().StartSpan(
		"connection",
		opentracing.ChildOf(c.span.Context()),
	)
	defer sp.Finish()
	defer c.span.Finish()
	for !c.dead {
		reqID := atomic.AddUint64(&c.requestCounter, 1)
		st := time.Now()
		l := make([]byte, 7)
		if n, err := c.Read(l); err != nil || n < 7 {
			c.logf("readNetPackets: short read: %v", err)
			c.dead = true
			return
		}
		sz := int64(l[0]) + int64(l[1])<<8 + int64(l[2])<<16 + int64(l[3])<<24
		t := MType(l[4])
		tag := uint16(l[5]) | uint16(l[6])<<8

		reqSpan := c.span.Tracer().StartSpan(
			fmt.Sprintf(fmt.Sprintf("%d", tag)),
			opentracing.ChildOf(sp.Context()),
			opentracing.Tag{"tag", tag},
			opentracing.Tag{"op", RPCNames[MType(l[4])]},
			opentracing.Tag{"requestID", reqID},
			opentracing.StartTime(st),
		)

		b := bytes.NewBuffer(l[5:])

		r := io.LimitReader(c.Reader, sz-7)
		if _, err := io.Copy(b, r); err != nil {
			c.logf("readNetPackets: short read: %v", err)
			c.dead = true
			return
		}
		c.logf("readNetPackets: got %v, len %d, sending to IO", RPCNames[MType(l[4])], b.Len())
		c.span.Tracer().StartSpan("read", opentracing.ChildOf(reqSpan.Context()), opentracing.StartTime(st)).Finish()

		if err := c.server.D(opentracing.ContextWithSpan(ctx, reqSpan), c.server, b, t); err != nil {
			c.logf("%v: %v", RPCNames[MType(l[4])], err)
		}

		c.logf("readNetPackets: Write %v back", b)

		writeSpan := c.span.Tracer().StartSpan("write", opentracing.ChildOf(reqSpan.Context()))
		amt, err := c.Write(b.Bytes())
		writeSpan.Finish()

		if err != nil {
			c.logf("readNetPackets: write error: %v", err)
			c.dead = true
			reqSpan.Finish()
			sp.Finish()
			return
		}
		c.logf("Returned %v amt %v", b, amt)
		reqSpan.Finish()
	}
}

// Dispatch dispatches request to different functions.
// It's also the the first place we try to establish server semantics.
// We could do this with interface assertions and such a la rsc/fuse
// but most people I talked do disliked that. So we don't. If you want
// to make things optional, just define the ones you want to implement in this case.
func Dispatch(ctx context.Context, s *Server, b *bytes.Buffer, t MType) error {
	switch t {
	case Tversion:
		s.Versioned = true
	default:
		if !s.Versioned {
			m := fmt.Sprintf("Dispatch: %v not allowed before Tversion", RPCNames[t])
			// Yuck. Provide helper.
			d := b.Bytes()
			MarshalRerrorPkt(b, Tag(d[0])|Tag(d[1])<<8, m)
			return fmt.Errorf("Dispatch: %v not allowed before Tversion", RPCNames[t])
		}
	}
	rpcSpan, ctx := opentracing.StartSpanFromContext(ctx, RPCNames[t])
	defer rpcSpan.Finish()
	switch t {
	case Tversion:
		return s.SrvRversion(ctx, b)
	case Tattach:
		return s.SrvRattach(ctx, b)
	case Tflush:
		return s.SrvRflush(ctx, b)
	case Twalk:
		return s.SrvRwalk(ctx, b)
	case Topen:
		return s.SrvRopen(ctx, b)
	case Tcreate:
		return s.SrvRcreate(ctx, b)
	case Tclunk:
		return s.SrvRclunk(ctx, b)
	case Tstat:
		return s.SrvRstat(ctx, b)
	case Twstat:
		return s.SrvRwstat(ctx, b)
	case Tremove:
		return s.SrvRremove(ctx, b)
	case Tread:
		return s.SrvRread(ctx, b)
	case Twrite:
		return s.SrvRwrite(ctx, b)
	}

	// This has been tested by removing Attach from the switch.
	ServerError(b, fmt.Sprintf("Dispatch: %v not supported", RPCNames[t]))
	return nil
}