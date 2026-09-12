package configaccess

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"testing"

	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
)

func TestScopedKeyAuthentication(t *testing.T) {
	p := newProvider("test", []string{"existing-1", "existing-2"})
	p.scopes = map[string]string{fmt.Sprintf("%x", sha256.Sum256([]byte("friend"))): "school.json"}
	for _, channel := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "key", "auth_token"} {
		t.Run(channel, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/responses", nil)
			if channel == "key" || channel == "auth_token" {
				r.URL.RawQuery = channel + "=friend"
			} else if channel == "Authorization" {
				r.Header.Set(channel, "Bearer friend")
			} else {
				r.Header.Set(channel, "friend")
			}
			result, err := p.Authenticate(context.Background(), r)
			if err != nil || result.Metadata[sdkaccess.ScopedAuthMetadataKey] != "school.json" {
				t.Fatalf("scope lost: result=%v err=%v", result, err)
			}
		})
	}
	for _, key := range []string{"existing-1", "existing-2"} {
		r := httptest.NewRequest("GET", "/v1/models", nil)
		r.Header.Set("Authorization", "Bearer "+key)
		result, err := p.Authenticate(context.Background(), r)
		if err != nil || result.Metadata[sdkaccess.ScopedAuthMetadataKey] != "" {
			t.Fatalf("existing key restricted: %v", err)
		}
	}
	r := httptest.NewRequest("GET", "/v1/models?key=friend", nil)
	r.Header.Set("Authorization", "Bearer existing-1")
	result, err := p.Authenticate(context.Background(), r)
	if err != nil || result.Metadata[sdkaccess.ScopedAuthMetadataKey] != "school.json" {
		t.Fatal("mixed credentials bypassed scope")
	}
	r = httptest.NewRequest("GET", "/v1/models", nil)
	r.Header.Set("Authorization", "Bearer friend")
	stock := newProvider("stock", []string{"existing-1", "existing-2"})
	if _, err := stock.Authenticate(context.Background(), r); err == nil {
		t.Fatal("stock configuration accepted scoped key")
	}
	p.scopes[fmt.Sprintf("%x", sha256.Sum256([]byte("friend")))] = ""
	if _, err := p.Authenticate(context.Background(), r); err == nil {
		t.Fatal("empty scope accepted")
	}
}
