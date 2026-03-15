package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// TestPagination_Users verifies pagination behavior on the users list endpoint.
func TestPagination_Users(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create several users so we have enough data to paginate.
	const count = 5
	createdIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		body := map[string]any{
			"username":   fmt.Sprintf("paguser%d", i),
			"email":      fmt.Sprintf("paguser%d@outpost.local", i),
			"password":   "PagTestP@ss1",
			"first_name": "Pag",
			"last_name":  fmt.Sprintf("User%d", i),
		}
		resp := authRequest(t, "POST", "/api/v1/users", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var user map[string]any
		decodeJSON(t, resp, &user)
		createdIDs = append(createdIDs, user["id"].(string))
	}
	t.Cleanup(func() {
		for _, id := range createdIDs {
			resp := authRequest(t, "DELETE", "/api/v1/users/"+id, nil, adminJWT)
			resp.Body.Close()
		}
	})

	t.Run("default pagination returns metadata", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Users   []map[string]any `json:"users"`
			Total   int              `json:"total"`
			Page    int              `json:"page"`
			PerPage int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if result.Page != 1 {
			t.Fatalf("expected page=1, got %d", result.Page)
		}
		if result.PerPage != 50 {
			t.Fatalf("expected per_page=50, got %d", result.PerPage)
		}
		// At least admin + the 5 we created.
		if result.Total < count+1 {
			t.Fatalf("expected total >= %d, got %d", count+1, result.Total)
		}
	})

	t.Run("per_page limits results", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users?per_page=2", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Users   []map[string]any `json:"users"`
			Total   int              `json:"total"`
			Page    int              `json:"page"`
			PerPage int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Users) != 2 {
			t.Fatalf("expected 2 users in page, got %d", len(result.Users))
		}
		if result.PerPage != 2 {
			t.Fatalf("expected per_page=2, got %d", result.PerPage)
		}
		// Total should still reflect all users, not just the page.
		if result.Total < count+1 {
			t.Fatalf("expected total >= %d, got %d", count+1, result.Total)
		}
	})

	t.Run("page parameter navigates pages", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users?per_page=2&page=2", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Users   []map[string]any `json:"users"`
			Total   int              `json:"total"`
			Page    int              `json:"page"`
			PerPage int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if result.Page != 2 {
			t.Fatalf("expected page=2, got %d", result.Page)
		}
		if len(result.Users) == 0 {
			t.Fatal("expected non-empty second page")
		}
	})

	t.Run("page beyond data returns empty list", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users?page=9999", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Users   []map[string]any `json:"users"`
			Total   int              `json:"total"`
			Page    int              `json:"page"`
			PerPage int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Users) != 0 {
			t.Fatalf("expected 0 users on far page, got %d", len(result.Users))
		}
		if result.Total < count+1 {
			t.Fatalf("total should still be correct; got %d", result.Total)
		}
	})

	t.Run("per_page capped at 100", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users?per_page=500", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			PerPage int `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if result.PerPage != 100 {
			t.Fatalf("expected per_page capped at 100, got %d", result.PerPage)
		}
	})
}

// TestPagination_Networks verifies pagination on the networks list endpoint.
func TestPagination_Networks(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create a few networks for pagination testing.
	const count = 3
	createdIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		body := map[string]any{
			"name":    fmt.Sprintf("pagnet%d", i),
			"address": fmt.Sprintf("10.%d.0.0/24", 200+i),
			"dns":     []string{"1.1.1.1"},
			"port":    51820 + i,
		}
		resp := authRequest(t, "POST", "/api/v1/networks", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var net map[string]any
		decodeJSON(t, resp, &net)
		createdIDs = append(createdIDs, net["id"].(string))
	}
	t.Cleanup(func() {
		for _, id := range createdIDs {
			resp := authRequest(t, "DELETE", "/api/v1/networks/"+id, nil, adminJWT)
			resp.Body.Close()
		}
	})

	t.Run("default pagination returns metadata", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/networks", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Networks []map[string]any `json:"networks"`
			Total    int              `json:"total"`
			Page     int              `json:"page"`
			PerPage  int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if result.Page != 1 {
			t.Fatalf("expected page=1, got %d", result.Page)
		}
		if result.PerPage != 50 {
			t.Fatalf("expected per_page=50, got %d", result.PerPage)
		}
		// At least the default + our created ones.
		if result.Total < count+1 {
			t.Fatalf("expected total >= %d, got %d", count+1, result.Total)
		}
	})

	t.Run("per_page limits results", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/networks?per_page=1", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Networks []map[string]any `json:"networks"`
			Total    int              `json:"total"`
			PerPage  int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Networks) != 1 {
			t.Fatalf("expected 1 network in page, got %d", len(result.Networks))
		}
		if result.PerPage != 1 {
			t.Fatalf("expected per_page=1, got %d", result.PerPage)
		}
	})

	t.Run("page beyond data returns empty list", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/networks?page=9999", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Networks []map[string]any `json:"networks"`
			Total    int              `json:"total"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Networks) != 0 {
			t.Fatalf("expected 0 networks on far page, got %d", len(result.Networks))
		}
	})
}

// TestPagination_Devices verifies pagination on the devices list endpoint.
func TestPagination_Devices(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	// Create several devices.
	const count = 3
	createdIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		body := map[string]any{
			"name":             fmt.Sprintf("pagdev%d", i),
			"wireguard_pubkey": "auto-generated",
			"user_id":          adminUserID,
		}
		resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var dev map[string]any
		decodeJSON(t, resp, &dev)
		createdIDs = append(createdIDs, dev["id"].(string))
	}
	t.Cleanup(func() {
		for _, id := range createdIDs {
			resp := authRequest(t, "DELETE", "/api/v1/devices/"+id, nil, adminJWT)
			resp.Body.Close()
		}
	})

	t.Run("default pagination returns metadata", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Devices []map[string]any `json:"devices"`
			Total   int              `json:"total"`
			Page    int              `json:"page"`
			PerPage int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if result.Page != 1 {
			t.Fatalf("expected page=1, got %d", result.Page)
		}
		if result.PerPage != 50 {
			t.Fatalf("expected per_page=50, got %d", result.PerPage)
		}
		if result.Total < count {
			t.Fatalf("expected total >= %d, got %d", count, result.Total)
		}
	})

	t.Run("per_page limits results", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices?per_page=1", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Devices []map[string]any `json:"devices"`
			PerPage int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Devices) != 1 {
			t.Fatalf("expected 1 device, got %d", len(result.Devices))
		}
	})

	t.Run("page beyond data returns empty list", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices?page=9999", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Devices []map[string]any `json:"devices"`
			Total   int              `json:"total"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Devices) != 0 {
			t.Fatalf("expected 0 devices on far page, got %d", len(result.Devices))
		}
	})
}

// TestPagination_Gateways verifies pagination on the gateways list endpoint.
func TestPagination_Gateways(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var networkID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE name = 'default'`).Scan(&networkID)
	if err != nil {
		t.Fatalf("failed to get default network id: %v", err)
	}

	// Create several gateways.
	const count = 3
	createdIDs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		body := map[string]any{
			"name":       fmt.Sprintf("paggw%d", i),
			"network_id": networkID,
			"endpoint":   fmt.Sprintf("gw-pag%d.test:51820", i),
		}
		resp := authRequest(t, "POST", "/api/v1/gateways", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var gw map[string]any
		decodeJSON(t, resp, &gw)
		createdIDs = append(createdIDs, gw["id"].(string))
	}
	t.Cleanup(func() {
		for _, id := range createdIDs {
			resp := authRequest(t, "DELETE", "/api/v1/gateways/"+id, nil, adminJWT)
			resp.Body.Close()
		}
	})

	t.Run("default pagination returns metadata", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/gateways", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Gateways []map[string]any `json:"gateways"`
			Total    int              `json:"total"`
			Page     int              `json:"page"`
			PerPage  int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if result.Page != 1 {
			t.Fatalf("expected page=1, got %d", result.Page)
		}
		if result.PerPage != 50 {
			t.Fatalf("expected per_page=50, got %d", result.PerPage)
		}
		if result.Total < count {
			t.Fatalf("expected total >= %d, got %d", count, result.Total)
		}
	})

	t.Run("per_page limits results", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/gateways?per_page=1", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Gateways []map[string]any `json:"gateways"`
			PerPage  int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Gateways) != 1 {
			t.Fatalf("expected 1 gateway, got %d", len(result.Gateways))
		}
	})

	t.Run("page beyond data returns empty list", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/gateways?page=9999", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Gateways []map[string]any `json:"gateways"`
			Total    int              `json:"total"`
		}
		decodeJSON(t, resp, &result)

		if len(result.Gateways) != 0 {
			t.Fatalf("expected 0 gateways on far page, got %d", len(result.Gateways))
		}
	})
}
