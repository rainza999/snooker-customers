package billingstatus

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestStatusWarnsWhenDueToday(t *testing.T) {
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "site.env")
	t.Setenv("SITE_ENV_PATH", path)
	today := time.Now().In(location).Format("2006-01-02")
	if err := os.WriteFile(path, []byte("BILLING_DUE_DATE="+today+"\nBILLING_ACTION=warn\nBILLING_STATUS=ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Get("/billing-status", Status)
	resp, err := app.Test(httptest.NewRequest("GET", "/billing-status", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled || body.Status != "warning" || !body.DueOrPast {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestStatusShutsDownWhenConfigured(t *testing.T) {
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "site.env")
	t.Setenv("SITE_ENV_PATH", path)
	today := time.Now().In(location).Format("2006-01-02")
	if err := os.WriteFile(path, []byte("BILLING_DUE_DATE="+today+"\nBILLING_ACTION=shutdown\nBILLING_STATUS=ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Get("/billing-status", Status)
	resp, err := app.Test(httptest.NewRequest("GET", "/billing-status", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Enabled || body.Status != "shutdown" {
		t.Fatalf("unexpected body: %+v", body)
	}
}
