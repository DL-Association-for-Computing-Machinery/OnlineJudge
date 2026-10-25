package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
)

func TestHealthIsProcessLivenessOnly(t *testing.T) {
	response := httptest.NewRecorder()
	httpapi.NewHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected health response: %d %v", response.Code, response.Header())
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["scope"] != "process" || payload["service"] != "api" || payload["status"] != "ok" {
		t.Fatalf("unexpected health contract: %v", payload)
	}
}

func TestUnimplementedRoutesNeverPretendSuccess(t *testing.T) {
	for _, tc := range []struct {
		method, path, code string
		status             int
	}{
		{http.MethodPost, "/api/v1/submissions", "NOT_FOUND", http.StatusNotFound},
		{http.MethodPost, "/api/v1/healthz", "METHOD_NOT_ALLOWED", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.path+tc.method, func(t *testing.T) {
			response := httptest.NewRecorder()
			httpapi.NewHandler().ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status || response.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("unexpected error response: %d %v", response.Code, response.Header())
			}
			var body struct {
				Code   string `json:"code"`
				Status int    `json:"status"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != tc.code || body.Status != tc.status {
				t.Fatalf("unexpected problem: %+v", body)
			}
		})
	}
}
