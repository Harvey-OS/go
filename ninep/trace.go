// Copyright 2020 The Ninep Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ninep

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"harvey-os.org/ninep/protocol"
)

func TraceNineServer(s protocol.NineServer) protocol.NineServer {
	return &tracer{s}
}

type tracer struct {
	s protocol.NineServer
}

func (t *tracer) Rversion(ctx context.Context, msize protocol.MaxSize, version string) (protocol.MaxSize, string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rversion",
		opentracing.Tag{"msize", msize},
		opentracing.Tag{"version", version},
	)
	defer span.Finish()
	return t.s.Rversion(ctx, msize, version)

}

func (t *tracer) Rattach(ctx context.Context, fid protocol.FID, afid protocol.FID, uname string, aname string) (protocol.QID, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rattach",
		opentracing.Tag{"fid", fid},
		opentracing.Tag{"afid", afid},
		opentracing.Tag{"aname", uname},
		opentracing.Tag{"aname", aname},
	)
	defer span.Finish()
	return t.s.Rattach(ctx, fid, afid, uname, aname)
}

func (t *tracer) Rwalk(ctx context.Context, fid protocol.FID, newfid protocol.FID, qids []string) ([]protocol.QID, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rwalk",
		opentracing.Tag{"fid", fid},
		opentracing.Tag{"newfid", newfid},
		opentracing.Tag{"qids", qids},
	)
	defer span.Finish()
	return t.s.Rwalk(ctx, fid, newfid, qids)
}

func (t *tracer) Ropen(ctx context.Context, fid protocol.FID, mode protocol.Mode) (protocol.QID, protocol.MaxSize, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Ropen",
		opentracing.Tag{"fid", fid},
		opentracing.Tag{"mode", mode},
	)
	defer span.Finish()
	return t.s.Ropen(ctx, fid, mode)
}

func (t *tracer) Rcreate(ctx context.Context, fid protocol.FID, name string, perm protocol.Perm, mode protocol.Mode) (protocol.QID, protocol.MaxSize, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rcreate",
		opentracing.Tag{"fid", fid},
		opentracing.Tag{"name", name},
		opentracing.Tag{"perm", perm},
		opentracing.Tag{"mode", mode},
	)
	defer span.Finish()
	return t.s.Rcreate(ctx, fid, name, perm, mode)
}

func (t *tracer) Rstat(ctx context.Context, fid protocol.FID) ([]byte, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rstat",
		opentracing.Tag{"fid", fid},
	)
	defer span.Finish()
	return t.s.Rstat(ctx, fid)
}

func (t *tracer) Rwstat(ctx context.Context, fid protocol.FID, b []byte) error {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rstat",
		opentracing.Tag{"fid", fid},
	)
	defer span.Finish()
	return t.s.Rwstat(ctx, fid, b)
}

func (t *tracer) Rclunk(ctx context.Context, fid protocol.FID) error {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rclunk",
		opentracing.Tag{"fid", fid},
	)
	defer span.Finish()
	return t.s.Rclunk(ctx, fid)
}

func (t *tracer) Rremove(ctx context.Context, fid protocol.FID) error {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rremove",
		opentracing.Tag{"fid", fid},
	)
	defer span.Finish()
	return t.s.Rremove(ctx, fid)
}

func (t *tracer) Rread(ctx context.Context, fid protocol.FID, offset protocol.Offset, count protocol.Count) ([]byte, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rread",
		opentracing.Tag{"fid", fid},
		opentracing.Tag{"offset", offset},
		opentracing.Tag{"count", count},
	)
	defer span.Finish()
	return t.s.Rread(ctx, fid, offset, count)
}

func (t *tracer) Rwrite(ctx context.Context, fid protocol.FID, offset protocol.Offset, b []byte) (protocol.Count, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rwrite",
		opentracing.Tag{"fid", fid},
		opentracing.Tag{"offset", offset},
	)
	defer span.Finish()
	return t.s.Rwrite(ctx, fid, offset, b)
}

func (t *tracer) Rflush(ctx context.Context, tag protocol.Tag) error {
	span, ctx := opentracing.StartSpanFromContext(ctx,
		"Rflush",
		opentracing.Tag{"tag", tag},
	)
	defer span.Finish()
	return t.s.Rflush(ctx, tag)
}
