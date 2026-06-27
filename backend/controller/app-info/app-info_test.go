package appinfo

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestInfoReturnsBuildMetadataAndReleaseNotes(t *testing.T) {
	tmp := t.TempDir()
	notesPath := filepath.Join(tmp, "release-notes.json")
	if err := os.WriteFile(notesPath, []byte(`{
		"window_days": 60,
		"generated": "2026-06-28",
		"notes": [
			{
				"date": "2026-06-28",
				"title": "Test release",
				"items": [{"commit": "abc123", "message": "Test item"}]
			}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write release notes: %v", err)
	}

	t.Setenv("APP_VERSION", "9.9.9")
	t.Setenv("APP_GIT_COMMIT", "abc123")
	t.Setenv("APP_GIT_BRANCH", "main")
	t.Setenv("APP_DEPLOY_DATE", "2026-06-28")
	t.Setenv("APP_SOURCE", "test")
	t.Setenv("APP_RELEASE_NOTES_PATH", notesPath)

	app := fiber.New()
	app.Get("/app-info", Info)

	req := httptest.NewRequest("GET", "/app-info", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request app info: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := body["version_label"]; got != "YS SNOOKER V9.9.9 (28/06/2026)" {
		t.Fatalf("unexpected version label: %v", got)
	}
	if got := body["commit"]; got != "abc123" {
		t.Fatalf("unexpected commit: %v", got)
	}
	notes, ok := body["release_notes"].([]any)
	if !ok || len(notes) != 1 {
		t.Fatalf("expected one release note, got %#v", body["release_notes"])
	}
}
