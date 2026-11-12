// White-box tests of the Host allow-list the composition root sets: ADR 0019
// requires it whenever the server listens on loopback without a token, which is
// the case DNS rebinding attacks, and requires an empty resolved list to be a
// startup error rather than a server that answers every Host.
package main

import (
	"net"
	"net/http"
	"testing"
)

// TestServerAllowedHostsOnLoopback pins the resolved allow-list: on loopback
// without a token the server answers to 127.0.0.1:<port> and localhost:<port>
// and refuses every other Host with 421, before it looks at the route or the
// body, so a page on an attacker's domain that resolves to 127.0.0.1 gets
// nothing. The health endpoints stay open, because a probe addresses the server
// by its own address.
func TestServerAllowedHostsOnLoopback(t *testing.T) {
	t.Parallel()

	s := startServer(t, loopbackListen, nil)

	tests := []struct {
		name string
		path string
		host string
		want int
	}{
		{name: "the loopback address with its port", path: "/v1/targets", host: "127.0.0.1:" + s.port, want: http.StatusOK},
		{name: "localhost with its port", path: "/v1/targets", host: "localhost:" + s.port, want: http.StatusOK},
		{name: "localhost in another case", path: "/v1/targets", host: "LocalHost:" + s.port, want: http.StatusOK},
		{name: "an attacker's name", path: "/v1/targets", host: "evil.example.com", want: http.StatusMisdirectedRequest},
		{name: "an attacker's name with the port", path: "/v1/targets", host: "evil.example.com:" + s.port, want: http.StatusMisdirectedRequest},
		{name: "another loopback address", path: "/v1/targets", host: "127.0.0.2:" + s.port, want: http.StatusMisdirectedRequest},
		{name: "healthz stays open to any host", path: "/healthz", host: "evil.example.com", want: http.StatusOK},
		{name: "readyz stays open to any host", path: "/readyz", host: "evil.example.com", want: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := newRequest(t, http.MethodGet, tc.path, nil)
			req.Host = tc.host

			got := s.do(t, req)

			if got.status != tc.want {
				t.Errorf("GET %s with Host %q = %d, want %d; body: %q",
					tc.path, tc.host, got.status, tc.want, got.body)
			}
		})
	}
}

// TestServerAllowedHostFlag pins --allowed-host: an entry replaces the resolved
// loopback list, so only the named Host is served, with its port or, as an entry
// without one, on any port.
func TestServerAllowedHostFlag(t *testing.T) {
	t.Parallel()

	s := startServer(t, loopbackListen, nil, "--allowed-host", "app.example.com")

	tests := []struct {
		host string
		want int
	}{
		{host: "app.example.com:" + s.port, want: http.StatusOK},
		{host: "app.example.com", want: http.StatusOK},
		{host: "127.0.0.1:" + s.port, want: http.StatusMisdirectedRequest},
		{host: "evil.example.com", want: http.StatusMisdirectedRequest},
	}

	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()

			req := newRequest(t, http.MethodGet, "/v1/targets", nil)
			req.Host = tc.host

			got := s.do(t, req)

			if got.status != tc.want {
				t.Errorf("GET /v1/targets with Host %q = %d, want %d; body: %q",
					tc.host, got.status, tc.want, got.body)
			}
		})
	}
}

// TestServerAllowedHostErrors pins the allow-list entries ADR 0019 refuses at
// startup: an entry that resolves to no host must never become "any host".
func TestServerAllowedHostErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "an empty entry", args: []string{"--allowed-host", ""}},
		{name: "an entry of white space", args: []string{"--allowed-host", "  "}},
		{name: "an empty entry beside a good one", args: []string{"--allowed-host", "app.example.com", "--allowed-host", ""}},
		{name: "an entry with no port but a colon", args: []string{"--allowed-host", "app.example.com:"}},
		{name: "an unbracketed IPv6 address", args: []string{"--allowed-host", "::1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := runOnce(t, nil, append([]string{"--listen", loopbackListen}, tc.args...)...)

			if got.listened {
				t.Fatalf("run listened on %v, want a startup error for %v", got.addr, tc.args)
			}

			requireStartupError(t, got)
		})
	}
}

// TestServerLoopbackBound pins the decision ADR 0019's bound-address check rests
// on (audit M1): a listener --listen named as loopback counts as loopback only
// when the address it bound is a loopback IP. The name localhost is resolved by
// the system, which may map it elsewhere, so the bound address decides; a
// listener not named as loopback stays non-loopback whatever it bound.
func TestServerLoopbackBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		named bool
		bound net.Addr
		want  bool
	}{
		{
			name:  "named loopback that bound IPv4 loopback",
			named: true,
			bound: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
			want:  true,
		},
		{
			name:  "named loopback that bound IPv6 loopback",
			named: true,
			bound: &net.TCPAddr{IP: net.ParseIP("::1"), Port: 8080},
			want:  true,
		},
		{
			name:  "named loopback that bound another loopback address",
			named: true,
			bound: &net.TCPAddr{IP: net.ParseIP("127.0.0.2"), Port: 8080},
			want:  true,
		},
		{
			name:  "named loopback that bound a documentation address",
			named: true,
			bound: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 8080},
		},
		{
			name:  "named loopback that bound every IPv4 address",
			named: true,
			bound: &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 8080},
		},
		{
			name:  "named loopback that bound no IP",
			named: true,
			bound: &net.TCPAddr{Port: 8080},
		},
		{
			name:  "named loopback that bound no TCP address",
			named: true,
			bound: &net.UnixAddr{Name: "/tmp/shift2eu.sock", Net: "unix"},
		},
		{
			name:  "named loopback with no bound address",
			named: true,
		},
		{
			name:  "not named as loopback that bound IPv4 loopback",
			named: false,
			bound: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := loopbackBound(tc.named, tc.bound); got != tc.want {
				t.Errorf("loopbackBound(named=%v, bound=%v) = %v, want %v",
					tc.named, tc.bound, got, tc.want)
			}
		})
	}
}
