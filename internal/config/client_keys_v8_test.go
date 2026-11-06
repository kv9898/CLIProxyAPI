package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClientKeyV8MigrationAndRevocation(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	legacy := "api-keys: [personal]\nscoped-api-keys:\n  " + digest + ": school.json\nclient-keys:\n  " + digest + ":\n    name: School\n    all: false\n    auth-ids: [school.json]\n"
	for _, raw := range []string{legacy, "access:\n" + "  " + strings.ReplaceAll(strings.TrimSuffix(legacy, "\n"), "\n", "\n  ") + "\n"} {
		cfg, err := ParseConfigBytes([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.ClientKeys[digest].AuthIDs) != 1 || cfg.ScopedAPIKeys[digest] != "school.json" {
			t.Fatal("key policies did not load")
		}
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err = os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if err = SaveConfigPreserveComments(path, cfg, true); err != nil {
			t.Fatal(err)
		}
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateV8Config(saved); err != nil {
			t.Fatal(err)
		}
		again, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(cfg.ClientKeys, again.ClientKeys) || !reflect.DeepEqual(cfg.ScopedAPIKeys, again.ScopedAPIKeys) || !reflect.DeepEqual(cfg.APIKeys, again.APIKeys) {
			t.Fatal("v8 migration changed key permissions")
		}
		delete(again.ClientKeys, digest)
		delete(again.ScopedAPIKeys, digest)
		if err = SaveConfigPreserveComments(path, again); err != nil {
			t.Fatal(err)
		}
		revoked, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(revoked.ClientKeys) != 0 || len(revoked.ScopedAPIKeys) != 0 {
			t.Fatal("revoked keys resurrected after v8 reload")
		}
	}
}
