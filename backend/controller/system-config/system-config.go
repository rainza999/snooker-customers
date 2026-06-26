package systemconfig

import (
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/relay"
)

type response struct {
	Required bool         `json:"required"`
	Status   relay.Status `json:"status"`
}

var (
	cacheMu      sync.Mutex
	cacheExpires time.Time
	cacheStatus  response
)

func Status(c *fiber.Ctx) error {
	return c.JSON(currentStatus(true))
}

func Save(c *fiber.Ctx) error {
	required := relayConfigRequired()
	if current := relay.CurrentStatus(false); current.Configured && current.Valid && !allowConfigUpdate() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "system config is already valid",
		})
	}

	var body relay.RuntimeConfig
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	status := relay.SaveRuntimeConfig(body)
	clearCache()
	if !status.Configured || !status.Valid {
		return c.Status(fiber.StatusBadRequest).JSON(response{
			Required: required,
			Status:   status,
		})
	}

	return c.JSON(response{
		Required: required,
		Status:   status,
	})
}

func Skip(c *fiber.Ctx) error {
	if current := relay.CurrentStatus(true); current.Configured && current.Valid && !allowConfigUpdate() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "system config is currently valid",
		})
	}

	if err := relay.DisableRuntimeConfig(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	clearCache()
	return c.JSON(currentStatus(false))
}

func RequireReady(c *fiber.Ctx) error {
	if isPublicPath(c.Path()) {
		return c.Next()
	}

	status := currentStatus(false)
	if !status.Required || status.Status.Valid {
		return c.Next()
	}

	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
		"error":  "system_config_required",
		"config": status,
	})
}

func currentStatus(validate bool) response {
	required := relayConfigRequired()
	if !required {
		status := relay.CurrentStatus(validate)
		if !status.Configured {
			status.Valid = true
		}
		return response{
			Required: false,
			Status:   status,
		}
	}

	if !validate {
		cacheMu.Lock()
		if time.Now().Before(cacheExpires) {
			cached := cacheStatus
			cacheMu.Unlock()
			return cached
		}
		cacheMu.Unlock()
	}

	result := response{
		Required: true,
		Status:   relay.CurrentStatus(true),
	}

	cacheMu.Lock()
	cacheStatus = result
	cacheExpires = time.Now().Add(30 * time.Second)
	cacheMu.Unlock()

	return result
}

func clearCache() {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cacheExpires = time.Time{}
	cacheStatus = response{}
}

func relayConfigRequired() bool {
	return relay.Required()
}

func allowConfigUpdate() bool {
	return relay.ConfigTruthy("SYSTEM_CONFIG_ALLOW_UPDATE")
}

func isPublicPath(path string) bool {
	publicPrefixes := []string{
		"/health",
		"/activate",
		"/license-status",
		"/machine-id",
		"/billing-status",
		"/system-config",
	}
	for _, prefix := range publicPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
