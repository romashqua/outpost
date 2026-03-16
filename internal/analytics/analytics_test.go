package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseTimeRange_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/bandwidth", nil)
	before := time.Now().UTC()
	from, to, err := parseTimeRange(req)
	after := time.Now().UTC()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "to" should be approximately now.
	if to.Before(before) || to.After(after) {
		t.Errorf("to=%v not in [%v, %v]", to, before, after)
	}

	// "from" should be approximately 24h before "to".
	diff := to.Sub(from)
	if diff < 23*time.Hour+59*time.Minute || diff > 24*time.Hour+1*time.Minute {
		t.Errorf("expected ~24h range, got %v", diff)
	}
}

func TestParseTimeRange_ExplicitValues(t *testing.T) {
	fromStr := "2025-06-01T00:00:00Z"
	toStr := "2025-06-02T12:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/bandwidth?from="+fromStr+"&to="+toStr, nil)

	from, to, err := parseTimeRange(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedFrom, _ := time.Parse(time.RFC3339, fromStr)
	expectedTo, _ := time.Parse(time.RFC3339, toStr)

	if !from.Equal(expectedFrom) {
		t.Errorf("from=%v, want %v", from, expectedFrom)
	}
	if !to.Equal(expectedTo) {
		t.Errorf("to=%v, want %v", to, expectedTo)
	}
}

func TestParseTimeRange_InvalidFrom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/bandwidth?from=not-a-date", nil)
	_, _, err := parseTimeRange(req)
	if err == nil {
		t.Fatal("expected error for invalid from, got nil")
	}
}

func TestParseTimeRange_InvalidTo(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/bandwidth?to=bad", nil)
	_, _, err := parseTimeRange(req)
	if err == nil {
		t.Fatal("expected error for invalid to, got nil")
	}
}

func TestParseTimeRange_OnlyFrom(t *testing.T) {
	fromStr := "2025-01-15T08:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/bandwidth?from="+fromStr, nil)
	from, to, err := parseTimeRange(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedFrom, _ := time.Parse(time.RFC3339, fromStr)
	if !from.Equal(expectedFrom) {
		t.Errorf("from=%v, want %v", from, expectedFrom)
	}
	// "to" should default to approximately now.
	if time.Since(to) > 2*time.Second {
		t.Errorf("to=%v should be close to now", to)
	}
}

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]int{"count": 42}
	respondJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type=%q, want application/json", ct)
	}

	var result map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["count"] != 42 {
		t.Errorf("count=%d, want 42", result["count"])
	}
}

func TestRespondJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["error"] != "something went wrong" {
		t.Errorf("error=%q, want %q", result["error"], "something went wrong")
	}
	if result["message"] != "something went wrong" {
		t.Errorf("message=%q, want %q", result["message"], "something went wrong")
	}
}

func TestFlowRecord_StructFields(t *testing.T) {
	now := time.Now()
	fr := FlowRecord{
		Timestamp: now,
		GatewayID: "gw-1",
		DeviceID:  "dev-1",
		UserID:    "user-1",
		SrcIP:     "10.0.0.1",
		DstIP:     "10.0.0.2",
		Protocol:  "tcp",
		DstPort:   443,
		BytesSent: 1024,
		BytesRecv: 2048,
		Duration:  5 * time.Second,
	}

	if fr.GatewayID != "gw-1" {
		t.Errorf("GatewayID=%q, want gw-1", fr.GatewayID)
	}
	if fr.Duration.Milliseconds() != 5000 {
		t.Errorf("Duration ms=%d, want 5000", fr.Duration.Milliseconds())
	}
}

func TestUserBandwidth_JSONSerialization(t *testing.T) {
	ub := UserBandwidth{
		UserID:   "u1",
		Username: "alice",
		RxBytes:  1000,
		TxBytes:  2000,
		Total:    3000,
	}

	data, err := json.Marshal(ub)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded UserBandwidth
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.UserID != "u1" {
		t.Errorf("UserID=%q, want u1", decoded.UserID)
	}
	if decoded.Total != 3000 {
		t.Errorf("Total=%d, want 3000", decoded.Total)
	}
}

func TestBandwidthBucket_JSONSerialization(t *testing.T) {
	bb := BandwidthBucket{
		Bucket:  time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		RxBytes: 500,
		TxBytes: 600,
	}

	data, err := json.Marshal(bb)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded BandwidthBucket
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.RxBytes != 500 {
		t.Errorf("RxBytes=%d, want 500", decoded.RxBytes)
	}
	if decoded.TxBytes != 600 {
		t.Errorf("TxBytes=%d, want 600", decoded.TxBytes)
	}
}

func TestHourlyConnections_JSONSerialization(t *testing.T) {
	hc := HourlyConnections{
		Hour:      14,
		DayOfWeek: 3,
		Count:     99,
	}

	data, err := json.Marshal(hc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]int
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded["hour"] != 14 {
		t.Errorf("hour=%d, want 14", decoded["hour"])
	}
	if decoded["day_of_week"] != 3 {
		t.Errorf("day_of_week=%d, want 3", decoded["day_of_week"])
	}
	if decoded["count"] != 99 {
		t.Errorf("count=%d, want 99", decoded["count"])
	}
}

func TestSummaryResponse_JSONFields(t *testing.T) {
	s := summaryResponse{
		TotalRxBytes:  100000,
		TotalTxBytes:  200000,
		TotalFlows:    50,
		UniqueUsers:   5,
		UniqueDevices: 10,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	expectedKeys := []string{"total_rx_bytes", "total_tx_bytes", "total_flows", "unique_users", "unique_devices"}
	for _, key := range expectedKeys {
		if _, ok := decoded[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}
