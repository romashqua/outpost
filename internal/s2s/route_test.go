package s2s

import "testing"

func TestRouteTable_Advertise(t *testing.T) {
	rt := NewRouteTable()

	rt.Advertise("gw-a", "keyA", "1.1.1.1:51820", []string{"10.0.1.0/24"}, 100)
	rt.Advertise("gw-b", "keyB", "2.2.2.2:51820", []string{"10.0.2.0/24"}, 100)

	all := rt.AllRoutes()
	if len(all) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(all))
	}

	route, ok := rt.BestRoute("10.0.1.0/24")
	if !ok {
		t.Fatal("expected route to 10.0.1.0/24")
	}
	if route.ViaGateway != "gw-a" {
		t.Errorf("expected via gw-a, got %s", route.ViaGateway)
	}
}

func TestRouteTable_BestRouteByMetric(t *testing.T) {
	rt := NewRouteTable()

	rt.Advertise("gw-a", "keyA", "1.1.1.1:51820", []string{"10.0.0.0/8"}, 200)
	rt.Advertise("gw-b", "keyB", "2.2.2.2:51820", []string{"10.0.0.0/8"}, 100)

	route, ok := rt.BestRoute("10.0.0.0/8")
	if !ok {
		t.Fatal("expected route")
	}
	if route.ViaGateway != "gw-b" {
		t.Errorf("expected gw-b (lower metric), got %s", route.ViaGateway)
	}
}

func TestRouteTable_Withdraw(t *testing.T) {
	rt := NewRouteTable()

	rt.Advertise("gw-a", "keyA", "1.1.1.1:51820", []string{"10.0.1.0/24"}, 100)
	rt.Advertise("gw-b", "keyB", "2.2.2.2:51820", []string{"10.0.2.0/24"}, 100)

	rt.Withdraw("gw-a")

	_, ok := rt.BestRoute("10.0.1.0/24")
	if ok {
		t.Error("expected no route to 10.0.1.0/24 after withdrawal")
	}

	_, ok = rt.BestRoute("10.0.2.0/24")
	if !ok {
		t.Error("expected route to 10.0.2.0/24 to remain")
	}
}

func TestRouteTable_RoutesForGateway(t *testing.T) {
	rt := NewRouteTable()

	rt.Advertise("gw-a", "keyA", "1.1.1.1:51820", []string{"10.0.1.0/24"}, 100)
	rt.Advertise("gw-b", "keyB", "2.2.2.2:51820", []string{"10.0.2.0/24"}, 100)

	routes := rt.RoutesForGateway("gw-a")
	if len(routes) != 1 {
		t.Fatalf("expected 1 route for gw-a, got %d", len(routes))
	}
	if routes[0].ViaGateway != "gw-b" {
		t.Errorf("expected route via gw-b, got %s", routes[0].ViaGateway)
	}
}
