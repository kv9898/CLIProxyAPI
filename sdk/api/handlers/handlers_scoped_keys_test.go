package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func schoolContext() context.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("accessMetadata", map[string]string{sdkaccess.ScopedAuthMetadataKey: "scoped-school"})
	return context.WithValue(context.Background(), "gin", c)
}

func TestScopedKeyOverridesSessionPins(t *testing.T) {
	for _, pin := range []string{"", "scoped-pro", "deepseek-old", "deepseek-new"} {
		ctx := WithPinnedAuthID(schoolContext(), pin)
		if got := requestExecutionMetadata(ctx)[coreexecutor.PinnedAuthMetadataKey]; got != "scoped-school" {
			t.Fatalf("pin %q escaped scope: %v", pin, got)
		}
	}
}

func TestManagedKeyCatalogUnionAndEmptySelection(t *testing.T) {
	r := registry.GetGlobalRegistry()
	r.RegisterClient("catalog-school", "codex", []*registry.ModelInfo{{ID: "school"}})
	r.RegisterClient("catalog-ds", "deepseek", []*registry.ModelInfo{{ID: "deepseek"}})
	defer r.UnregisterClient("catalog-school")
	defer r.UnregisterClient("catalog-ds")
	body := []byte(`{"data":[{"id":"school"},{"id":"deepseek"},{"id":"pro"}]}`)
	out, err := filterAllowedModelList(body, []string{"catalog-school", "catalog-ds"})
	if err != nil || strings.Contains(string(out), `"pro"`) || !strings.Contains(string(out), `"deepseek"`) || !strings.Contains(string(out), `"school"`) {
		t.Fatalf("invalid union: %s %v", out, err)
	}
	out, err = filterAllowedModelList(body, nil)
	if err != nil || string(out) != `{"data":[]}` {
		t.Fatalf("empty selection exposed models: %s", out)
	}
}

type scopedExecutor struct {
	provider string
	calls    []string
	fail     bool
}

func (e *scopedExecutor) Identifier() string { return e.provider }
func (e *scopedExecutor) Execute(_ context.Context, a *coreauth.Auth, _ coreexecutor.Request, _ coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls = append(e.calls, a.ID)
	if e.fail {
		return coreexecutor.Response{}, &coreauth.Error{Code: "quota_exceeded", Message: "school quota exhausted", HTTPStatus: 429, Retryable: true}
	}
	return coreexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
}
func (e *scopedExecutor) ExecuteStream(_ context.Context, a *coreauth.Auth, _ coreexecutor.Request, _ coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	e.calls = append(e.calls, a.ID)
	ch := make(chan coreexecutor.StreamChunk, 1)
	if e.fail {
		ch <- coreexecutor.StreamChunk{Err: &coreauth.Error{Code: "upstream_error", Message: "school unavailable", HTTPStatus: 503, Retryable: true}}
	} else {
		ch <- coreexecutor.StreamChunk{Payload: []byte("data: [DONE]\n\n")}
	}
	close(ch)
	return &coreexecutor.StreamResult{Chunks: ch}, nil
}
func (e *scopedExecutor) CountTokens(ctx context.Context, a *coreauth.Auth, r coreexecutor.Request, o coreexecutor.Options) (coreexecutor.Response, error) {
	return e.Execute(ctx, a, r, o)
}
func (e *scopedExecutor) Refresh(_ context.Context, a *coreauth.Auth) (*coreauth.Auth, error) {
	return a, nil
}
func (e *scopedExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	panic("direct HTTP path must be blocked")
}

func TestScopedKeyExecutionIsolation(t *testing.T) {
	testKeyExecutionIsolation(t, schoolContext)
}

func TestManagedKeyExecutionIsolation(t *testing.T) {
	testKeyExecutionIsolation(t, func() context.Context {
		ctx := schoolContext()
		c := ctx.Value("gin").(*gin.Context)
		// Multiple allowed IDs, including a missing one; no legacy pin fallback.
		c.Set("accessMetadata", map[string]string{sdkaccess.AllowedAuthMetadataKey: `["scoped-school","missing"]`})
		return ctx
	})
}

func testKeyExecutionIsolation(t *testing.T, requestContext func() context.Context) {
	for _, mode := range []string{"success", "quota", "stream-failure", "missing", "disabled", "deepseek", "deepseek-old"} {
		t.Run(mode, func(t *testing.T) {
			manager := coreauth.NewManager(nil, nil, nil)
			codex := &scopedExecutor{provider: "codex", fail: mode == "quota" || mode == "stream-failure"}
			deepseek := &scopedExecutor{provider: "deepseek"}
			old := &scopedExecutor{provider: "deepseek-old"}
			for _, e := range []*scopedExecutor{codex, deepseek, old} {
				manager.RegisterExecutor(e)
			}
			for _, a := range []*coreauth.Auth{
				{ID: "scoped-school", Provider: "codex", Status: coreauth.StatusActive, Disabled: mode == "disabled"},
				{ID: "scoped-pro", Provider: "codex", Status: coreauth.StatusActive},
				{ID: "scoped-ds", Provider: "deepseek", Status: coreauth.StatusActive},
				{ID: "scoped-ds-old", Provider: "deepseek-old", Status: coreauth.StatusActive},
			} {
				if a.ID == "scoped-school" && mode == "missing" {
					continue
				}
				if _, err := manager.Register(context.Background(), a); err != nil {
					t.Fatal(err)
				}
				models := []*registry.ModelInfo{{ID: "scoped-shared"}}
				if a.Provider != "codex" {
					models = append(models, &registry.ModelInfo{ID: a.Provider + "-only"})
				}
				registry.GetGlobalRegistry().RegisterClient(a.ID, a.Provider, models)
				t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(a.ID) })
			}
			h := NewBaseAPIHandlers(&sdkconfig.SDKConfig{Streaming: sdkconfig.StreamingConfig{BootstrapRetries: 2}}, manager)
			ctx := requestContext()
			// A stale/disallowed session pin must not expand the credential allowlist.
			if scopedAuthIDFromGin(ctx.Value("gin").(*gin.Context)) != "" {
				ctx = WithPinnedAuthID(ctx, "scoped-pro")
			}
			model := "scoped-shared"
			if strings.HasPrefix(mode, "deepseek") {
				model = mode + "-only"
			}
			if mode == "stream-failure" {
				data, _, errs := h.ExecuteStreamWithAuthManager(ctx, "openai", model, []byte(`{"model":"scoped-shared"}`), "")
				for range data {
				}
				hadError := false
				for err := range errs {
					hadError = hadError || err != nil
				}
				if !hadError {
					t.Fatal("expected terminal stream error")
				}
			} else {
				_, _, err := h.ExecuteWithAuthManager(ctx, "openai", model, []byte(`{"model":"`+model+`"}`), "")
				if (err == nil) != (mode == "success") {
					t.Fatalf("unexpected result for %s: %v", mode, err)
				}
			}
			if len(deepseek.calls)+len(old.calls) != 0 {
				t.Fatal("DeepSeek executed")
			}
			for _, id := range codex.calls {
				if id != "scoped-school" {
					t.Fatalf("escaped to %s", id)
				}
			}
			if (mode == "success" || mode == "quota" || mode == "stream-failure") && len(codex.calls) == 0 {
				t.Fatal("school not exercised")
			}
		})
	}
}

func TestScopedKeyModelCatalog(t *testing.T) {
	r := registry.GetGlobalRegistry()
	r.RegisterClient("scoped-school", "codex", []*registry.ModelInfo{{ID: "school-model"}})
	t.Cleanup(func() { r.UnregisterClient("scoped-school") })
	for _, body := range []string{
		`{"object":"list","data":[{"id":"school-model","context":100},{"id":"pro-only"},{"id":"deepseek"}]}`,
		`{"models":[{"slug":"school-model","context":100},{"slug":"pro-only"},{"slug":"deepseek"}]}`,
	} {
		out, err := filterScopedModelList([]byte(body), "scoped-school")
		if err != nil || !json.Valid(out) || strings.Contains(string(out), "pro-only") || strings.Contains(string(out), "deepseek") || !strings.Contains(string(out), "school-model") || !strings.Contains(string(out), "100") {
			t.Fatalf("catalog not scoped: %s %v", out, err)
		}
		out, err = filterScopedModelList([]byte(body), "missing-school")
		if err != nil || strings.Contains(string(out), "school-model") {
			t.Fatalf("missing auth leaked models: %s %v", out, err)
		}
	}
}
