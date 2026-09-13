package openai

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/tidwall/gjson"
)

func TestScopedKeyWebsocketCannotSwitchAccounts(t *testing.T) {
	testKeyWebsocketIsolation(t, map[string]string{sdkaccess.ScopedAuthMetadataKey: "ws-school"})
}

func TestManagedKeyWebsocketCannotSwitchAccounts(t *testing.T) {
	testKeyWebsocketIsolation(t, map[string]string{sdkaccess.AllowedAuthMetadataKey: `["ws-school"]`})
}

func testKeyWebsocketIsolation(t *testing.T, metadata map[string]string) {
	codex := &websocketDirectCaptureExecutor{provider: "codex"}
	ds := &websocketDirectCaptureExecutor{provider: "deepseek"}
	old := &websocketDirectCaptureExecutor{provider: "deepseek-old"}
	manager := coreauth.NewManager(nil, &orderedWebsocketSelector{order: []string{"ws-pro", "ws-school", "ws-ds", "ws-ds-old"}}, nil)
	for _, e := range []*websocketDirectCaptureExecutor{codex, ds, old} {
		manager.RegisterExecutor(e)
	}
	for _, entry := range []struct{ id, provider, model string }{
		{"ws-school", "codex", "ws-school-model"}, {"ws-pro", "codex", "ws-school-model"},
		{"ws-ds", "deepseek", "ws-deepseek"}, {"ws-ds-old", "deepseek-old", "ws-deepseek-old"},
	} {
		a := &coreauth.Auth{ID: entry.id, Provider: entry.provider, Status: coreauth.StatusActive, Attributes: map[string]string{"websockets": "true"}}
		if _, err := manager.Register(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		registry.GetGlobalRegistry().RegisterClient(a.ID, a.Provider, []*registry.ModelInfo{{ID: entry.model}})
		t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(a.ID) })
	}
	h := NewOpenAIResponsesAPIHandler(handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager))
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		c.Set("accessMetadata", metadata)
		c.Next()
	}, h.ResponsesWebsocket)
	server := httptest.NewServer(router)
	defer server.Close()
	for _, forbiddenModel := range []string{"ws-deepseek", "ws-deepseek-old"} {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		for _, model := range []string{"ws-school-model", forbiddenModel} {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"response.create","model":%q,"input":[]}`, model))); err != nil {
				t.Fatal(err)
			}
			_, payload, err := conn.ReadMessage()
			// Upstream intentionally closes some terminal errors without an error frame.
			if err != nil && model == forbiddenModel {
				break
			}
			if err != nil {
				t.Fatalf("model %s: %v; codex=%v deepseek=%v old=%v", model, err, codex.AuthIDs(), ds.AuthIDs(), old.AuthIDs())
			}
			kind := gjson.GetBytes(payload, "type").String()
			if model == "ws-school-model" && kind != wsEventTypeCompleted {
				t.Fatalf("school request failed: %s", payload)
			}
			if model != "ws-school-model" && kind != "error" {
				t.Fatalf("provider switch succeeded: %s", payload)
			}
		}
	}
	if len(ds.AuthIDs())+len(old.AuthIDs()) != 0 {
		t.Fatal("DeepSeek executed over websocket")
	}
	ids := codex.AuthIDs()
	if len(ids) != 2 {
		t.Fatalf("unexpected school calls: %v", ids)
	}
	for _, id := range ids {
		if id != "ws-school" {
			t.Fatalf("Pro executed over websocket: %v", ids)
		}
	}
}
