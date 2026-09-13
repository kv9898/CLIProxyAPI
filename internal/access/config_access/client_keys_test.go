package configaccess

import (
	"context"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/clientkeys"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"net/http/httptest"
	"testing"
)

func TestManagedKeysAuthenticateAndFailClosed(t *testing.T) {
	p := newProvider("test", []string{"normal-one", "normal-two", "restricted"})
	p.policies = map[string]string{clientkeys.ID("restricted"): `["school"]`, clientkeys.ID("empty"): `[]`, clientkeys.ID("all"): ""}
	for _, channel := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "key", "auth_token"} {
		r := httptest.NewRequest("POST", "/v1/responses", nil)
		if channel == "key" || channel == "auth_token" {
			r.URL.RawQuery = channel + "=restricted"
		} else {
			r.Header.Set(channel, "restricted")
		}
		result, err := p.Authenticate(context.Background(), r)
		if err != nil || result.Metadata[sdkaccess.AllowedAuthMetadataKey] != `["school"]` {
			t.Fatalf("channel %s lost restriction", channel)
		}
	}
	for _, key := range []string{"normal-one", "normal-two", "all", "empty"} {
		r := httptest.NewRequest("GET", "/v1/models", nil)
		r.Header.Set("Authorization", "Bearer "+key)
		result, err := p.Authenticate(context.Background(), r)
		if err != nil {
			t.Fatalf("%s rejected", key)
		}
		if key == "empty" && result.Metadata[sdkaccess.AllowedAuthMetadataKey] != "[]" {
			t.Fatal("empty policy became unrestricted")
		}
	}
	r := httptest.NewRequest("GET", "/v1/models?key=restricted", nil)
	r.Header.Set("Authorization", "Bearer normal-one")
	result, err := p.Authenticate(context.Background(), r)
	if err != nil || result.Metadata[sdkaccess.AllowedAuthMetadataKey] == "" {
		t.Fatal("mixed credentials bypassed policy")
	}
}
