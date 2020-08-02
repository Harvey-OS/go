// UFS is a userspace server which exports a filesystem over 9p2000.
//
// By default, it will export / over a TCP on port 5640 under the username
// of "harvey".
package main

import (
	"flag"
	"io"
	"log"
	"net"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-lib/metrics"
	"harvey-os.org/ninep/protocol"
	"harvey-os.org/ninep/ufs"

	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
)

var (
	ntype = flag.String("net", "tcp4", "Default network type")
	naddr = flag.String("addr", ":5640", "Network address")
	debug = flag.Int("debug", 0, "print debug messages")
	root  = flag.String("root", "/", "Set the root for all attaches")
	trace = flag.Bool("trace", false, "Trace stuff")
)

func main() {
	flag.Parse()
	if *trace {
		closer := setupTracer()
		defer closer.Close()
	}
	ln, err := net.Listen(*ntype, *naddr)
	if err != nil {
		log.Fatalf("Listen failed: %v", err)
	}

	ufslistener, err := ufs.NewUFS(*root, *debug, func(l *protocol.NetListener) error {
		l.Trace = nil
		if *debug > 1 {
			l.Trace = log.Printf
		}
		return nil
	})
	if err := ufslistener.Serve(ln); err != nil {
		log.Fatal(err)
	}
}

func setupTracer() io.Closer {

	cfg := jaegercfg.Configuration{
		ServiceName: "ufs",
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: true,
		},
	}

	jLogger := jaegerlog.StdLogger
	jMetricsFactory := metrics.NullFactory

	// Initialize tracer with a logger and a metrics factory
	tracer, closer, err := cfg.NewTracer(
		jaegercfg.Logger(jLogger),
		jaegercfg.Metrics(jMetricsFactory),
	)
	if err != nil {
		panic(err)
	}
	// Set the singleton opentracing.Tracer with the Jaeger tracer.
	opentracing.SetGlobalTracer(tracer)
	return closer

}
