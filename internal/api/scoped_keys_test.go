package api

import (
	"net/http/httptest"
	"testing"
)

func TestScopedKeyRouteAllowlist(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		allow        bool
	}{
		{"GET", "/v1/models", true}, {"POST", "/v1/responses", true},
		{"GET", "/v1/responses", true}, {"GET", "/backend-api/codex/responses", true},
		{"POST", "/v1/responses/compact", true}, {"POST", "/v1/chat/completions", true},
		{"POST", "/v1/images/generations", false}, {"POST", "/v1/alpha/search", false},
		{"POST", "/v1/realtime/client_secrets", false}, {"POST", "/v1/live", false},
		{"GET", "/openai/v1/videos/secret/content", false}, {"GET", "/v1beta/models", false},
		{"POST", "/api/provider/openai/v1/chat/completions", false},
		{"POST", "/v1/responses/../images/generations", false}, {"GET", "/v0/management/config", false},
	} {
		if got := scopedClientRouteAllowed(httptest.NewRequest(tc.method, tc.path, nil)); got != tc.allow {
			t.Errorf("%s %s allowed=%v", tc.method, tc.path, got)
		}
	}
}
