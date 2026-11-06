package auth

import (
	"context"
	"testing"

	executor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func TestClientKeyAllowlistEligibility(t *testing.T) {
	for _, ids := range []any{[]string{"school", "deepseek-old"}, []string{}, nil, "malformed"} {
		e := authSelectionEligibilityForRequest(context.Background(), executor.Options{Metadata: map[string]any{executor.AllowedAuthIDsMetadataKey: ids}})
		if e.allows(&Auth{ID: "pro"}) || e.allows(&Auth{ID: "deepseek"}) {
			t.Fatal("unselected credential allowed")
		}
		values, _ := ids.([]string)
		if e.allows(&Auth{ID: "school"}) != (len(values) > 0) || e.allows(&Auth{ID: "deepseek-old"}) != (len(values) > 0) {
			t.Fatal("allowlist not honored")
		}
	}
	e := authSelectionEligibilityForRequest(context.Background(), executor.Options{})
	if !e.allows(&Auth{ID: "pro"}) {
		t.Fatal("unrestricted key changed")
	}
}
