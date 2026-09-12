package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
)

func scopedAuthIDFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	value, exists := c.Get("accessMetadata")
	if !exists {
		return ""
	}
	metadata, _ := value.(map[string]string)
	return metadata[sdkaccess.ScopedAuthMetadataKey]
}

// Preserve both OpenAI and Codex catalog shapes, including model capabilities.
func filterScopedModelList(body []byte, authID string) ([]byte, error) {
	allowed := make(map[string]bool)
	for _, model := range registry.GetGlobalRegistry().GetModelsForClient(authID) {
		if model != nil {
			allowed[model.ID] = true
		}
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	found := false
	for _, field := range []string{"data", "models"} {
		raw, exists := payload[field]
		if !exists {
			continue
		}
		found = true
		var models []json.RawMessage
		if err := json.Unmarshal(raw, &models); err != nil {
			return nil, err
		}
		filtered := make([]json.RawMessage, 0, len(models))
		for _, model := range models {
			var identity struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(model, &identity); err != nil {
				return nil, err
			}
			id := identity.ID
			if id == "" {
				id = identity.Slug
			}
			if allowed[id] {
				filtered = append(filtered, model)
			}
		}
		payload[field], _ = json.Marshal(filtered)
	}
	if !found {
		return nil, fmt.Errorf("unrecognized catalog shape")
	}
	return json.Marshal(payload)
}
