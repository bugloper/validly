// Command fakedns is a tiny DNS server with fixed answers, used by the
// end-to-end tests so they never depend on public DNS.
//
//	deliverable.validly-e2e.net  MX 10 mx.validly-e2e.net
//	nullmx.validly-e2e.net       MX 0 .            (RFC 7505)
//	fallback.validly-e2e.net     A 93.184.215.14   (no MX)
//	private.validly-e2e.net      A 10.0.0.1        (no MX)
//	servfail.validly-e2e.net     SERVFAIL
//	*.validly-e2e.net            NXDOMAIN
//	anything else                REFUSED
package main

import (
	"flag"
	"log"
	"strings"

	"github.com/miekg/dns"
)

const zone = "validly-e2e.net."

func main() {
	addr := flag.String("addr", "127.0.0.1:5354", "UDP and TCP listen address")
	flag.Parse()

	dns.HandleFunc(".", handle)
	errc := make(chan error, 2)
	for _, network := range []string{"udp", "tcp"} {
		srv := &dns.Server{Addr: *addr, Net: network}
		go func() { errc <- srv.ListenAndServe() }()
	}
	log.Printf("fakedns listening on %s", *addr)
	log.Fatal(<-errc)
}

func handle(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)
	m.Authoritative = true
	q := req.Question[0]
	name := strings.ToLower(q.Name)
	soa, _ := dns.NewRR(zone + " 60 IN SOA ns." + zone + " admin." + zone + " 1 60 60 60 60")
	rr := func(s string) dns.RR { r, _ := dns.NewRR(s); return r }

	switch {
	case !strings.HasSuffix(name, "."+zone) && name != zone:
		m.Rcode = dns.RcodeRefused
	case name == "servfail."+zone:
		m.Rcode = dns.RcodeServerFailure
	case name == "deliverable."+zone && q.Qtype == dns.TypeMX:
		m.Answer = append(m.Answer, rr(name+" 300 IN MX 10 mx."+zone))
	case name == "nullmx."+zone && q.Qtype == dns.TypeMX:
		m.Answer = append(m.Answer, rr(name+" 300 IN MX 0 ."))
	case name == "fallback."+zone && q.Qtype == dns.TypeA:
		m.Answer = append(m.Answer, rr(name+" 300 IN A 93.184.215.14"))
	case name == "private."+zone && q.Qtype == dns.TypeA:
		m.Answer = append(m.Answer, rr(name+" 300 IN A 10.0.0.1"))
	case name == "deliverable."+zone, name == "nullmx."+zone, name == "fallback."+zone,
		name == "private."+zone, name == "mx."+zone:
		m.Ns = append(m.Ns, soa) // NODATA
	default:
		m.Rcode = dns.RcodeNameError
		m.Ns = append(m.Ns, soa)
	}
	_ = w.WriteMsg(m)
}
