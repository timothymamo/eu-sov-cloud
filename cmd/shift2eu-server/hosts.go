package main

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

// isLoopbackListen reports whether the host part of a --listen address names
// loopback only: localhost or a loopback IP address. An empty host, an
// unspecified address (0.0.0.0, ::) and any other name listen beyond the
// machine, or may, so they are not loopback.
func isLoopbackListen(addr string) (bool, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false, err
	}

	if strings.EqualFold(host, "localhost") {
		return true, nil
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback(), nil
}

// loopbackBound reports whether a listener that --listen named as loopback
// (named) bound a loopback address: a name such as localhost is resolved by
// the system, which may map it elsewhere, so the bound address decides (ADR
// 0019). A listener not named as loopback stays non-loopback.
func loopbackBound(named bool, bound net.Addr) bool {
	tcp, ok := bound.(*net.TCPAddr)

	return named && ok && tcp.IP != nil && tcp.IP.IsLoopback()
}

// errNoLoopbackHosts is returned when the allow-list resolved for a loopback
// listener is empty: it must never become "any host" (ADR 0019).
var errNoLoopbackHosts = errors.New("the allowed host list resolved for the loopback listener is empty")

// loopbackHosts returns the Host values a loopback listener on addr answers to:
// localhost and the address it bound, each with the bound port. A browser
// sends one of them; a page on another domain that resolves to the loopback
// address sends its own name, which the list refuses (DNS rebinding).
func loopbackHosts(addr net.Addr) ([]string, error) {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsLoopback() || tcp.Port == 0 {
		return nil, errNoLoopbackHosts
	}

	port := strconv.Itoa(tcp.Port)

	return []string{
		net.JoinHostPort("localhost", port),
		net.JoinHostPort(tcp.IP.String(), port),
	}, nil
}

// stringList is a flag that may be repeated, each value appended.
type stringList []string

// String returns the values, comma separated, as flag.Value.
func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

// Set appends value, as flag.Value.
func (l *stringList) Set(value string) error {
	*l = append(*l, value)

	return nil
}
