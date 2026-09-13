package management

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/clientkeys"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestClientKeyManagementMigration(t *testing.T) {
	cfg := &config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"normal-one", "normal-two"}, ScopedAPIKeys: map[string]string{clientkeys.ID("friend"): "school.json"}}}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveConfigPreserveComments(path, cfg); err != nil {
		t.Fatal(err)
	}
	h := &Handler{cfg: cfg, configFilePath: path}
	router := gin.New()
	router.GET("/client-keys", h.GetClientKeys)
	router.PUT("/client-keys/:id", h.PutClientKey)
	router.POST("/client-keys", h.PutClientKey)
	router.DELETE("/client-keys/:id", h.DeleteClientKey)
	call := func(method, url, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/client-keys", "")
	var result struct {
		Keys []clientKeyView `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Keys) != 3 || strings.Contains(w.Body.String(), "normal-one") {
		t.Fatal("list missing keys or exposing secrets")
	}
	id := clientkeys.ID("friend")
	w = call("PUT", "/client-keys/"+id, `{"name":"Friend","all":false,"auth_ids":["school.json"]}`)
	if w.Code != 200 {
		t.Fatalf("save failed: %s", w.Body)
	}
	saved, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.APIKeys) != 2 || len(saved.ScopedAPIKeys) != 0 || saved.ClientKeys[id].All || len(saved.ClientKeys[id].AuthIDs) != 1 {
		t.Fatalf("migration lost isolation or modified existing keys: normal=%d scoped=%v managed=%+v", len(saved.APIKeys), saved.ScopedAPIKeys, saved.ClientKeys)
	}
	w = call("PUT", "/client-keys/"+clientkeys.ID("normal-one"), `{"name":"Personal","all":true,"auth_ids":[]}`)
	if w.Code != 200 || !h.cfg.ClientKeys[clientkeys.ID("normal-one")].All {
		t.Fatal("unrestricted key changed")
	}
	if h.clientKeysLocked()[clientkeys.ID("normal-two")].Name != "Key 2" {
		t.Fatal("remaining legacy key was renamed by index shift")
	}
	w = call("POST", "/client-keys", `{"key":"synthetic-new-key-for-testing-only","name":"New","all":false,"auth_ids":[]}`)
	if w.Code != 200 {
		t.Fatalf("create failed: %s", w.Body)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "synthetic-new-key-for-testing-only") {
		t.Fatal("new secret persisted in plaintext")
	}
	w = call("DELETE", "/client-keys/"+id, "")
	if w.Code != 200 {
		t.Fatal("delete failed")
	}
	if _, exists := h.clientKeysLocked()[id]; exists {
		t.Fatal("deleted key remains")
	}
	saved, err = config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := saved.ClientKeys[id]; exists {
		t.Fatal("deleted key resurrected after reload")
	}
	if _, exists := saved.ScopedAPIKeys[id]; exists {
		t.Fatal("legacy deleted key resurrected after reload")
	}
}

func TestClientKeySaveFailureDoesNotMutate(t *testing.T) {
	cfg := &config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"only-key"}}}
	h := &Handler{cfg: cfg, configFilePath: filepath.Join(t.TempDir(), "missing", "config.yaml")}
	router := gin.New()
	router.PUT("/client-keys/:id", h.PutClientKey)
	router.DELETE("/client-keys/:id", h.DeleteClientKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("PUT", "/client-keys/"+clientkeys.ID("only-key"), strings.NewReader(`{"name":"Test","all":false,"auth_ids":[]}`)))
	if w.Code != 500 || len(cfg.APIKeys) != 1 || len(cfg.ClientKeys) != 0 {
		t.Fatal("failed save changed active configuration")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("DELETE", "/client-keys/"+clientkeys.ID("only-key"), nil))
	if w.Code != 409 {
		t.Fatal("last key deletion could enable public access")
	}
}
