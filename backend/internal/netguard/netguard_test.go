package netguard

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestBlockRefusesTheAgentNetwork(t *testing.T) {
	g := &Guard{subnets: []netip.Prefix{netip.MustParsePrefix("172.30.0.0/16")}, gateways: []netip.Addr{netip.MustParseAddr("172.30.0.1")}}
	h := g.Block(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))

	for addr, want := range map[string]int{
		"172.30.0.5:51234": http.StatusForbidden,
		"172.30.0.1:51234": http.StatusTeapot, // the network's gateway: the host, through the published port
		"172.18.0.1:40000": http.StatusTeapot, // compose network / published port
		"127.0.0.1:5000":   http.StatusTeapot,
		"[::1]:5000":       http.StatusTeapot,
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s: status %d, want %d", addr, rec.Code, want)
		}
	}
}
