package management

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/clientkeys"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

type clientKeyView struct {
	ID string `json:"id"`
	config.ClientKeyPolicy
}

// clientKeysLocked merges legacy credentials without ever exposing their secrets.
func (h *Handler) clientKeysLocked() map[string]config.ClientKeyPolicy {
	keys := make(map[string]config.ClientKeyPolicy)
	for i, key := range h.cfg.APIKeys {
		keys[clientkeys.ID(key)] = config.ClientKeyPolicy{Name: fmt.Sprintf("Key %d", i+1), All: true, AuthIDs: []string{}}
	}
	for id, auth := range h.cfg.ScopedAPIKeys {
		keys[id] = config.ClientKeyPolicy{Name: "School key", AuthIDs: []string{auth}}
	}
	for id, policy := range h.cfg.ClientKeys {
		keys[id] = policy
	}
	return keys
}

func (h *Handler) GetClientKeys(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	keys := h.clientKeysLocked()
	result := make([]clientKeyView, 0, len(keys))
	for id, policy := range keys {
		result = append(result, clientKeyView{id, policy})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	type resource struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
		Disabled bool   `json:"disabled"`
	}
	resources := make([]resource, 0)
	if h.authManager != nil {
		for _, auth := range h.authManager.List() {
			if auth == nil {
				continue
			}
			name := auth.FileName
			if name == "" {
				name = auth.Label
			}
			if name == "" {
				name = auth.ID
			}
			resources = append(resources, resource{auth.ID, name, auth.Provider, auth.Disabled})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })
	c.JSON(200, gin.H{"keys": result, "resources": resources, "usage": clientkeys.Default.Snapshot(), "recording": h.cfg.UsageStatisticsEnabled})
}

func (h *Handler) PutClientKey(c *gin.Context) {
	var body struct {
		config.ClientKeyPolicy
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid key policy"})
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || len(body.Name) > 100 {
		c.JSON(400, gin.H{"error": "name must be 1–100 bytes"})
		return
	}
	id := c.Param("id")
	creating := id == ""
	if creating {
		if len(body.Key) < 24 || len(body.Key) > 256 || strings.TrimSpace(body.Key) != body.Key {
			c.JSON(400, gin.H{"error": "key must be 24–256 characters without surrounding whitespace"})
			return
		}
		id = clientkeys.ID(body.Key)
	}
	digest, err := hex.DecodeString(id)
	if err != nil || len(digest) != 32 {
		c.JSON(400, gin.H{"error": "invalid key id"})
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, exists := h.clientKeysLocked()[id]
	if creating && exists {
		c.JSON(409, gin.H{"error": "key already exists"})
		return
	}
	if !creating && !exists {
		c.JSON(404, gin.H{"error": "key not found"})
		return
	}
	// Retain missing IDs (e.g. temporarily removed files) as deny-only references.
	ids := make([]string, 0, len(body.AuthIDs))
	seen := make(map[string]bool)
	if len(body.AuthIDs) > 1000 {
		c.JSON(400, gin.H{"error": "too many credentials"})
		return
	}
	for _, authID := range body.AuthIDs {
		if authID == "" || len(authID) > 1024 {
			c.JSON(400, gin.H{"error": "invalid credential id"})
			return
		}
		if !seen[authID] {
			ids = append(ids, authID)
			seen[authID] = true
		}
	}
	body.AuthIDs = ids
	oldKeys, oldScoped, oldManaged := h.cfg.APIKeys, h.cfg.ScopedAPIKeys, h.cfg.ClientKeys
	h.removeClientKeyLocked(id)
	h.cfg.ClientKeys[id] = body.ClientKeyPolicy
	if !h.persistLocked(c) {
		h.cfg.APIKeys, h.cfg.ScopedAPIKeys, h.cfg.ClientKeys = oldKeys, oldScoped, oldManaged
	}
}

func (h *Handler) removeClientKeyLocked(id string) {
	views := h.clientKeysLocked()
	keys := make([]string, 0, len(h.cfg.APIKeys))
	for _, key := range h.cfg.APIKeys {
		if clientkeys.ID(key) != id {
			keys = append(keys, key)
		}
	}
	scoped := make(map[string]string)
	for digest, auth := range h.cfg.ScopedAPIKeys {
		if digest != id {
			scoped[digest] = auth
		}
	}
	managed := make(map[string]config.ClientKeyPolicy)
	for digest, policy := range h.cfg.ClientKeys {
		if digest != id {
			managed[digest] = policy
		}
	}
	// Preserve the displayed names of remaining legacy keys when array indexes shift.
	// Their raw credentials and effective permissions are unchanged.
	for _, key := range keys {
		digest := clientkeys.ID(key)
		if _, exists := managed[digest]; !exists {
			managed[digest] = views[digest]
		}
	}
	h.cfg.APIKeys, h.cfg.ScopedAPIKeys, h.cfg.ClientKeys = keys, scoped, managed
}

func (h *Handler) DeleteClientKey(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := c.Param("id")
	keys := h.clientKeysLocked()
	if _, exists := keys[id]; !exists {
		c.JSON(404, gin.H{"error": "key not found"})
		return
	}
	// An empty auth configuration historically means public access; never create it here.
	if len(keys) <= 1 {
		c.JSON(409, gin.H{"error": "cannot delete the last client key"})
		return
	}
	oldKeys, oldScoped, oldManaged := h.cfg.APIKeys, h.cfg.ScopedAPIKeys, h.cfg.ClientKeys
	h.removeClientKeyLocked(id)
	if !h.persistLocked(c) {
		h.cfg.APIKeys, h.cfg.ScopedAPIKeys, h.cfg.ClientKeys = oldKeys, oldScoped, oldManaged
	}
}
