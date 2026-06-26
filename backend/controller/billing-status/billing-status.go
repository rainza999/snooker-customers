package billingstatus

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type statusResponse struct {
	Enabled     bool   `json:"enabled"`
	Status      string `json:"status"`
	DueDate     string `json:"due_date"`
	Cycle       string `json:"cycle"`
	Action      string `json:"action"`
	Message     string `json:"message"`
	DueOrPast   bool   `json:"due_or_past"`
	DaysOverdue int    `json:"days_overdue"`
}

func Status(c *fiber.Ctx) error {
	values := readSiteEnv()
	dueDate := firstValue(values, "BILLING_DUE_DATE")
	cycle := valueOr(firstValue(values, "BILLING_CYCLE"), "monthly")
	action := strings.ToLower(valueOr(firstValue(values, "BILLING_ACTION"), "warn"))
	status := strings.ToLower(valueOr(firstValue(values, "BILLING_STATUS"), "ok"))
	message := firstValue(values, "BILLING_MESSAGE")

	dueOrPast, daysOverdue := dueState(dueDate)
	if status == "ok" && dueOrPast {
		if action == "shutdown" {
			status = "shutdown"
		} else {
			status = "warning"
		}
	}
	if message == "" && status == "warning" {
		message = "กรุณาชำระค่าบริการระบบ"
	}
	if message == "" && status == "shutdown" {
		message = "ระบบถูกระงับชั่วคราว กรุณาติดต่อผู้ดูแล"
	}

	return c.JSON(statusResponse{
		Enabled:     status != "ok",
		Status:      status,
		DueDate:     dueDate,
		Cycle:       cycle,
		Action:      action,
		Message:     message,
		DueOrPast:   dueOrPast,
		DaysOverdue: daysOverdue,
	})
}

func dueState(value string) (bool, int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return false, 0
	}
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		location = time.Local
	}
	due, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return false, 0
	}
	now := time.Now().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	diffDays := int(today.Sub(due).Hours() / 24)
	return !due.After(today), maxInt(diffDays, 0)
}

func readSiteEnv() map[string]string {
	values := map[string]string{}
	data, err := os.ReadFile(siteEnvPath())
	if err != nil {
		return values
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values
}

func siteEnvPath() string {
	if path := strings.TrimSpace(os.Getenv("SITE_ENV_PATH")); path != "" {
		return path
	}
	return filepath.Join("config", "site.env")
}

func firstValue(fileValues map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fileValues[key]); value != "" {
			return value
		}
	}
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
