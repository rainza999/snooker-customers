package appinfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type ReleaseItem struct {
	Commit  string `json:"commit"`
	Message string `json:"message"`
}

type ReleaseDay struct {
	Date  string        `json:"date"`
	Title string        `json:"title"`
	Items []ReleaseItem `json:"items"`
}

type releaseNotesFile struct {
	WindowDays int          `json:"window_days"`
	Generated  string       `json:"generated"`
	Notes      []ReleaseDay `json:"notes"`
}

var processStartedAt = time.Now()

func Info(c *fiber.Ctx) error {
	version := envOrDefault("APP_VERSION", "2.0.1")
	commit := envOrDefault("APP_GIT_COMMIT", "unknown")
	branch := envOrDefault("APP_GIT_BRANCH", "main")
	source := envOrDefault("APP_SOURCE", "server")
	deployDate := strings.TrimSpace(os.Getenv("APP_DEPLOY_DATE"))
	if deployDate == "" {
		deployDate = processStartedAt.Format("2006-01-02")
	}

	return c.JSON(fiber.Map{
		"product":             "YS SNOOKER",
		"version":             version,
		"version_label":       "YS SNOOKER V" + version + " (" + displayDate(deployDate) + ")",
		"commit":              commit,
		"branch":              branch,
		"source":              source,
		"deploy_date":         deployDate,
		"deploy_date_display": displayDate(deployDate),
		"release_notes":       loadReleaseNotes(),
	})
}

func loadReleaseNotes() []ReleaseDay {
	path := strings.TrimSpace(os.Getenv("APP_RELEASE_NOTES_PATH"))
	if path == "" {
		path = "release-notes.json"
	}

	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, path)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return []ReleaseDay{}
	}

	var file releaseNotesFile
	if err := json.Unmarshal(content, &file); err != nil {
		return []ReleaseDay{}
	}

	return file.Notes
}

func displayDate(value string) string {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("02/01/2006")
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
