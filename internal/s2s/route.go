package s2s

import (
	"sort"
	"sync"
)

// RouteEntry represents a route in the S2S route table.
type RouteEntry struct {
	Destination  string
	ViaGateway   string
	ViaPublicKey string
	ViaEndpoint  string
	Metric       int
	IsActive     bool
}

// RouteTable maintains the global S2S routing table.
// Each gateway advertises its local subnets. The route table
// computes the best path to each destination.
type RouteTable struct {
	mu     sync.RWMutex
	routes map[string][]RouteEntry // key: destination CIDR
}

func NewRouteTable() *RouteTable {
	return &RouteTable{
		routes: make(map[string][]RouteEntry),
	}
}

// Advertise adds or updates routes from a gateway.
func (rt *RouteTable) Advertise(gatewayID, publicKey, endpoint string, subnets []string, metric int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	for _, subnet := range subnets {
		entry := RouteEntry{
			Destination:  subnet,
			ViaGateway:   gatewayID,
			ViaPublicKey: publicKey,
			ViaEndpoint:  endpoint,
			Metric:       metric,
			IsActive:     true,
		}

		routes := rt.routes[subnet]

		// Update existing or append.
		found := false
		for i, r := range routes {
			if r.ViaGateway == gatewayID {
				routes[i] = entry
				found = true
				break
			}
		}
		if !found {
			routes = append(routes, entry)
		}

		// Sort by metric (lower is better).
		sort.Slice(routes, func(i, j int) bool {
			return routes[i].Metric < routes[j].Metric
		})

		rt.routes[subnet] = routes
	}
}

// Withdraw removes all routes from a gateway.
func (rt *RouteTable) Withdraw(gatewayID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	for dest, routes := range rt.routes {
		filtered := routes[:0]
		for _, r := range routes {
			if r.ViaGateway != gatewayID {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(rt.routes, dest)
		} else {
			rt.routes[dest] = filtered
		}
	}
}

// BestRoute returns the best route (lowest metric) to a destination.
func (rt *RouteTable) BestRoute(destination string) (RouteEntry, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	routes, ok := rt.routes[destination]
	if !ok || len(routes) == 0 {
		return RouteEntry{}, false
	}

	for _, r := range routes {
		if r.IsActive {
			return r, true
		}
	}
	return RouteEntry{}, false
}

// AllRoutes returns a snapshot of the entire route table.
func (rt *RouteTable) AllRoutes() []RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var all []RouteEntry
	for _, routes := range rt.routes {
		all = append(all, routes...)
	}
	return all
}

// RoutesForGateway returns the routes that a specific gateway should install.
// This is all routes NOT via itself.
func (rt *RouteTable) RoutesForGateway(gatewayID string) []RouteEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var result []RouteEntry
	for _, routes := range rt.routes {
		for _, r := range routes {
			if r.ViaGateway != gatewayID && r.IsActive {
				result = append(result, r)
				break // Only best route per destination.
			}
		}
	}
	return result
}
