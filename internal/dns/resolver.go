package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"

	mdns "github.com/miekg/dns"
)

// Resolver is a Querier that sends queries to recursive resolvers using
// github.com/miekg/dns. The standard library resolver is not used because it
// does not distinguish NXDOMAIN from NODATA or expose record TTLs.
type Resolver struct {
	servers []string
	timeout time.Duration
	udp     *mdns.Client
	tcp     *mdns.Client
}

// NewResolver returns a Resolver for the given "host:port" servers. If none
// are given, the system resolvers from /etc/resolv.conf are used. queryTimeout
// bounds each individual query attempt.
func NewResolver(servers []string, queryTimeout time.Duration) (*Resolver, error) {
	if len(servers) == 0 {
		conf, err := mdns.ClientConfigFromFile("/etc/resolv.conf")
		if err != nil {
			return nil, fmt.Errorf("read system resolvers: %w", err)
		}
		for _, s := range conf.Servers {
			servers = append(servers, net.JoinHostPort(s, conf.Port))
		}
	}
	if len(servers) == 0 {
		return nil, errors.New("no DNS servers configured")
	}
	for i, s := range servers {
		if _, _, err := net.SplitHostPort(s); err != nil {
			servers[i] = net.JoinHostPort(s, "53")
		}
	}
	return &Resolver{
		servers: servers,
		timeout: queryTimeout,
		udp:     &mdns.Client{Net: "udp", Timeout: queryTimeout, UDPSize: 1232},
		tcp:     &mdns.Client{Net: "tcp", Timeout: queryTimeout},
	}, nil
}

// Servers returns the upstream resolvers in use.
func (r *Resolver) Servers() []string { return r.servers }

// Query sends one query, trying each server in turn until one returns a
// response that is not SERVFAIL or REFUSED.
func (r *Resolver) Query(ctx context.Context, name string, qtype QType) (Answer, error) {
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(name), uint16(qtype))
	msg.RecursionDesired = true
	msg.SetEdns0(1232, false)

	var (
		last    Answer
		lastErr error
		gotResp bool
	)
	for _, server := range r.servers {
		if err := ctx.Err(); err != nil {
			break
		}
		resp, err := r.exchange(ctx, msg, server)
		if err != nil {
			lastErr = err
			continue
		}
		last, gotResp = parse(resp, qtype), true
		if last.Rcode != RcodeServFail && last.Rcode != RcodeRefused {
			return last, nil
		}
	}
	if gotResp {
		return last, nil
	}
	if lastErr == nil {
		lastErr = ctx.Err()
	}
	return Answer{}, lastErr
}

func (r *Resolver) exchange(ctx context.Context, msg *mdns.Msg, server string) (*mdns.Msg, error) {
	resp, _, err := r.udp.ExchangeContext(ctx, msg, server)
	if err == nil && resp.Truncated {
		resp, _, err = r.tcp.ExchangeContext(ctx, msg, server)
	}
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() || errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %s", ErrTimeout, server)
		}
		return nil, err
	}
	if resp.Id != msg.Id {
		return nil, errors.New("dns response id mismatch")
	}
	return resp, nil
}

func parse(resp *mdns.Msg, qtype QType) Answer {
	a := Answer{}
	switch resp.Rcode {
	case mdns.RcodeSuccess:
		a.Rcode = RcodeSuccess
	case mdns.RcodeNameError:
		a.Rcode = RcodeNXDomain
	case mdns.RcodeServerFailure:
		a.Rcode = RcodeServFail
	case mdns.RcodeRefused:
		a.Rcode = RcodeRefused
	default:
		a.Rcode = RcodeOther
	}

	var minTTL uint32
	seen := false
	note := func(ttl uint32) {
		if !seen || ttl < minTTL {
			minTTL, seen = ttl, true
		}
	}
	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *mdns.MX:
			if qtype == TypeMX {
				host := strings.ToLower(strings.TrimSuffix(v.Mx, "."))
				a.MX = append(a.MX, MX{Host: host, Pref: v.Preference})
				note(v.Hdr.Ttl)
			}
		case *mdns.A:
			if qtype == TypeA {
				if addr, ok := netip.AddrFromSlice(v.A.To4()); ok {
					a.Addrs = append(a.Addrs, addr)
					note(v.Hdr.Ttl)
				}
			}
		case *mdns.AAAA:
			if qtype == TypeAAAA {
				if addr, ok := netip.AddrFromSlice(v.AAAA); ok {
					a.Addrs = append(a.Addrs, addr)
					note(v.Hdr.Ttl)
				}
			}
		}
	}
	// RFC 2308: negative answers are cached for min(SOA TTL, SOA MINIMUM).
	if !seen {
		for _, rr := range resp.Ns {
			if soa, ok := rr.(*mdns.SOA); ok {
				note(min(soa.Hdr.Ttl, soa.Minttl))
			}
		}
	}
	if seen {
		a.TTL = time.Duration(minTTL) * time.Second
	}
	return a
}
