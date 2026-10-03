// Package netguard keeps agent containers away from the app's own API.
//
// Agent containers live on their own Docker network, shared only with api so they can reach the
// inference proxy. api also serves the UI API on another port, which controls Docker; requests
// to it from the agent network are refused.
package netguard

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"

	"github.com/moby/moby/client"
)

type Guard struct {
	subnets []netip.Prefix
}

// ForNetwork reads the subnets of a Docker network.
func ForNetwork(ctx context.Context, cli *client.Client, name string) (*Guard, error) {
	res, err := cli.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("inspecting network %s: %w", name, err)
	}
	g := &Guard{}
	for _, c := range res.Network.IPAM.Config {
		if c.Subnet.IsValid() {
			g.subnets = append(g.subnets, c.Subnet)
		}
	}
	if len(g.subnets) == 0 {
		return nil, fmt.Errorf("network %s has no subnets", name)
	}
	return g, nil
}

// Contains reports whether a remote address ("ip:port") belongs to the guarded network.
func (g *Guard) Contains(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, s := range g.subnets {
		if s.Contains(ip) {
			return true
		}
	}
	return false
}

// Block wraps a handler and refuses requests coming from the guarded network.
func (g *Guard) Block(next http.Handler) http.Handler {
	if g == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.Contains(r.RemoteAddr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "agent containers cannot use the ai-compare API"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Guard) Subnets() []netip.Prefix { return g.subnets }
