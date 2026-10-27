package dns

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	mdns "github.com/miekg/dns"
)

// startServer runs a local DNS server on 127.0.0.1 so the Resolver can be
// tested without touching public DNS.
func startServer(t *testing.T, handler mdns.HandlerFunc) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &mdns.Server{PacketConn: pc, Handler: handler}
	started := make(chan struct{})
	srv.NotifyStartedFunc = func() { close(started) }
	go func() { _ = srv.ActivateAndServe() }()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

func zone(t *testing.T) mdns.HandlerFunc {
	return func(w mdns.ResponseWriter, req *mdns.Msg) {
		m := new(mdns.Msg)
		m.SetReply(req)
		q := req.Question[0]
		soa, _ := mdns.NewRR("example.net. 900 IN SOA ns. host. 1 7200 3600 1209600 120")
		switch {
		case q.Name == "mail.example.net." && q.Qtype == mdns.TypeMX:
			rr1, _ := mdns.NewRR("mail.example.net. 600 IN MX 10 MX1.Example.net.")
			rr2, _ := mdns.NewRR("mail.example.net. 300 IN MX 20 mx2.example.net.")
			m.Answer = append(m.Answer, rr1, rr2)
		case q.Name == "null.example.net." && q.Qtype == mdns.TypeMX:
			rr, _ := mdns.NewRR("null.example.net. 3600 IN MX 0 .")
			m.Answer = append(m.Answer, rr)
		case q.Name == "a.example.net." && q.Qtype == mdns.TypeA:
			rr, _ := mdns.NewRR("a.example.net. 60 IN A 93.184.215.14")
			m.Answer = append(m.Answer, rr)
		case q.Name == "fail.example.net.":
			m.Rcode = mdns.RcodeServerFailure
		case q.Name == "big.example.net." && q.Qtype == mdns.TypeMX:
			if _, udp := w.RemoteAddr().(*net.UDPAddr); udp {
				m.Truncated = true
			} else {
				rr, _ := mdns.NewRR("big.example.net. 60 IN MX 5 tcp.example.net.")
				m.Answer = append(m.Answer, rr)
			}
		case q.Name == "a.example.net.", q.Name == "big.example.net.":
			m.Ns = append(m.Ns, soa)
		default:
			m.Rcode = mdns.RcodeNameError
			m.Ns = append(m.Ns, soa)
		}
		_ = w.WriteMsg(m)
	}
}

func TestResolverParsesResponses(t *testing.T) {
	addr := startServer(t, zone(t))
	// Serve TCP on the same port for the truncation test.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	tcpSrv := &mdns.Server{Listener: ln, Handler: zone(t)}
	go func() { _ = tcpSrv.ActivateAndServe() }()
	t.Cleanup(func() { _ = tcpSrv.Shutdown() })

	r, err := NewResolver([]string{addr}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	a, err := r.Query(ctx, "mail.example.net", TypeMX)
	if err != nil || a.Rcode != RcodeSuccess || len(a.MX) != 2 || a.MX[0].Host != "mx1.example.net" || a.TTL != 300*time.Second {
		t.Fatalf("mx: %+v %v", a, err)
	}

	a, _ = r.Query(ctx, "null.example.net", TypeMX)
	if len(a.MX) != 1 || a.MX[0].Host != "" {
		t.Fatalf("null mx: %+v", a)
	}

	a, _ = r.Query(ctx, "a.example.net", TypeMX)
	if a.Rcode != RcodeSuccess || len(a.MX) != 0 || a.TTL != 120*time.Second {
		t.Fatalf("nodata: %+v", a)
	}

	a, _ = r.Query(ctx, "missing.example.net", TypeMX)
	if a.Rcode != RcodeNXDomain || a.TTL != 120*time.Second {
		t.Fatalf("nxdomain: %+v", a)
	}

	a, _ = r.Query(ctx, "fail.example.net", TypeMX)
	if a.Rcode != RcodeServFail {
		t.Fatalf("servfail: %+v", a)
	}

	a, err = r.Query(ctx, "big.example.net", TypeMX)
	if err != nil || len(a.MX) != 1 || a.MX[0].Host != "tcp.example.net" {
		t.Fatalf("truncated retry over tcp: %+v %v", a, err)
	}
}

func TestResolverFallsBackToNextServer(t *testing.T) {
	failing := startServer(t, func(w mdns.ResponseWriter, req *mdns.Msg) {
		m := new(mdns.Msg)
		m.SetRcode(req, mdns.RcodeServerFailure)
		_ = w.WriteMsg(m)
	})
	good := startServer(t, zone(t))
	r, _ := NewResolver([]string{failing, good}, time.Second)
	a, err := r.Query(context.Background(), "mail.example.net", TypeMX)
	if err != nil || len(a.MX) != 2 {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestResolverTimeout(t *testing.T) {
	silent := startServer(t, func(w mdns.ResponseWriter, req *mdns.Msg) {})
	r, _ := NewResolver([]string{silent}, 100*time.Millisecond)
	_, err := r.Query(context.Background(), "mail.example.net", TypeMX)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestEndToEndWithLocalServer(t *testing.T) {
	addr := startServer(t, zone(t))
	r, _ := NewResolver([]string{addr}, time.Second)
	c := NewChecker(r, nil, DefaultConfig(), nil, nil)
	ctx := context.Background()
	for domain, want := range map[string]Reason{
		"mail.example.net":    ReasonMX,
		"null.example.net":    ReasonNullMX,
		"a.example.net":       ReasonAddressFallback,
		"missing.example.net": ReasonNXDomain,
		"fail.example.net":    ReasonServFail,
	} {
		if got := c.Check(ctx, domain); got.Reason != want {
			t.Errorf("%s: got %+v, want %s", domain, got, want)
		}
	}
}

// A dead first resolver must leave time to try the next one.
func TestFailoverWithinLookupBudget(t *testing.T) {
	silent := startServer(t, func(w mdns.ResponseWriter, req *mdns.Msg) {})
	good := startServer(t, zone(t))
	cfg := DefaultConfig()
	cfg.Timeout = 900 * time.Millisecond
	r, _ := NewResolver([]string{silent, good}, cfg.Timeout/3)
	got := NewChecker(r, nil, cfg, nil, nil).Check(context.Background(), "a.example.net")
	if got.Status != StatusDeliverable || got.Reason != ReasonAddressFallback {
		t.Fatalf("got %+v", got)
	}
}
