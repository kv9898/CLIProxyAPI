package configaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// Register ensures the config-access provider is available to the access manager.
func Register(cfg *sdkconfig.SDKConfig) {
	if cfg == nil {
		sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey)
		return
	}

	keys := normalizeKeys(cfg.APIKeys)
	if len(keys) == 0 && len(cfg.ScopedAPIKeys) == 0 && len(cfg.ClientKeys) == 0 {
		sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey)
		return
	}

	p := newProvider(sdkaccess.DefaultAccessProviderName, keys)
	p.policies = make(map[string]string, len(cfg.ClientKeys))
	for digest, policy := range cfg.ClientKeys {
		encoded := ""
		if !policy.All {
			data, _ := json.Marshal(policy.AuthIDs)
			encoded = string(data)
		}
		p.policies[digest] = encoded
	}
	p.scopes = make(map[string]string, len(cfg.ScopedAPIKeys))
	for digest, authID := range cfg.ScopedAPIKeys {
		digest = strings.ToLower(strings.TrimSpace(digest))
		decoded, errDecode := hex.DecodeString(digest)
		if errDecode == nil && len(decoded) == sha256.Size {
			p.scopes[digest] = strings.TrimSpace(authID)
		}
	}
	sdkaccess.RegisterProvider(
		sdkaccess.AccessProviderTypeConfigAPIKey,
		p,
	)
}

type provider struct {
	policies map[string]string
	name     string
	keys     map[string]struct{}
	scopes   map[string]string
}

func newProvider(name string, keys []string) *provider {
	providerName := strings.TrimSpace(name)
	if providerName == "" {
		providerName = sdkaccess.DefaultAccessProviderName
	}
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	return &provider{name: providerName, keys: keySet}
}

func (p *provider) Identifier() string {
	if p == nil || p.name == "" {
		return sdkaccess.DefaultAccessProviderName
	}
	return p.name
}

func (p *provider) Authenticate(_ context.Context, r *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	if p == nil {
		return nil, sdkaccess.NewNotHandledError()
	}
	if len(p.keys) == 0 && len(p.scopes) == 0 && len(p.policies) == 0 {
		return nil, sdkaccess.NewNotHandledError()
	}
	authHeader := r.Header.Get("Authorization")
	authHeaderGoogle := r.Header.Get("X-Goog-Api-Key")
	authHeaderAnthropic := r.Header.Get("X-Api-Key")
	queryKey := ""
	queryAuthToken := ""
	if r.URL != nil {
		queryKey = r.URL.Query().Get("key")
		queryAuthToken = r.URL.Query().Get("auth_token")
	}
	if authHeader == "" && authHeaderGoogle == "" && authHeaderAnthropic == "" && queryKey == "" && queryAuthToken == "" {
		return nil, sdkaccess.NewNoCredentialsError()
	}

	apiKey := extractBearerToken(authHeader)

	candidates := []struct {
		value  string
		source string
	}{
		{apiKey, "authorization"},
		{authHeaderGoogle, "x-goog-api-key"},
		{authHeaderAnthropic, "x-api-key"},
		{queryKey, "query-key"},
		{queryAuthToken, "query-auth-token"},
	}

	// A scoped credential takes precedence even if multiple auth channels are supplied.
	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		digest := sha256.Sum256([]byte(candidate.value))
		if allowed, ok := p.policies[hex.EncodeToString(digest[:])]; ok && allowed != "" {
			return &sdkaccess.Result{Provider: p.Identifier(), Principal: candidate.value,
				Metadata: map[string]string{"source": candidate.source, sdkaccess.AllowedAuthMetadataKey: allowed}}, nil
		}
	}
	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		digest := sha256.Sum256([]byte(candidate.value))
		if authID, ok := p.scopes[hex.EncodeToString(digest[:])]; ok {
			if authID == "" {
				return nil, sdkaccess.NewInvalidCredentialError()
			}
			return &sdkaccess.Result{
				Provider: p.Identifier(), Principal: candidate.value,
				Metadata: map[string]string{"source": candidate.source, sdkaccess.ScopedAuthMetadataKey: authID},
			}, nil
		}
	}
	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		if _, ok := p.keys[candidate.value]; ok {
			return &sdkaccess.Result{
				Provider:  p.Identifier(),
				Principal: candidate.value,
				Metadata: map[string]string{
					"source": candidate.source,
				},
			}, nil
		}
	}

	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		digest := sha256.Sum256([]byte(candidate.value))
		if allowed, ok := p.policies[hex.EncodeToString(digest[:])]; ok && allowed == "" {
			return &sdkaccess.Result{Provider: p.Identifier(), Principal: candidate.value, Metadata: map[string]string{"source": candidate.source}}, nil
		}
	}

	return nil, sdkaccess.NewInvalidCredentialError()
}

func extractBearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return header
	}
	if strings.ToLower(parts[0]) != "bearer" {
		return header
	}
	return strings.TrimSpace(parts[1])
}

func normalizeKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if _, exists := seen[trimmedKey]; exists {
			continue
		}
		seen[trimmedKey] = struct{}{}
		normalized = append(normalized, trimmedKey)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
