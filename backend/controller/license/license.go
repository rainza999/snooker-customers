package license

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"

	"crypto/rand"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
)

type ActivateRequest struct {
	ActivationKey string `json:"activation_key"`
	MachineID     string `json:"machine_id"`
}

func ActivateLicense(c *fiber.Ctx) error {
	type ReqBody struct {
		ActivationKey string `json:"activation_key"`
		MachineID     string `json:"machine_id"`
	}
	var bodyx ReqBody
	if err := c.BodyParser(&bodyx); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	var key model.ActivationKey
	if err := db.Db.Where("key = ? AND is_used = false", bodyx.ActivationKey).First(&key).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Key not found or already used"})
	}

	now := time.Now()
	key.IsUsed = true
	key.MachineID = &bodyx.MachineID
	key.UsedAt = &now

	if err := db.Db.Save(&key).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update key"})
	}

	// ✅ สร้าง license.json
	license := fiber.Map{
		"license": fiber.Map{
			"machine_id":   bodyx.MachineID,
			"activated_at": now.Format(time.RFC3339),
			"is_valid":     true, // ✅ เพิ่มบรรทัดนี้
		},
	}
	data, _ := json.MarshalIndent(license, "", "  ")

	// ❗ สำคัญ: ใช้ path ที่ Docker container เขียนได้
	// licensePath := "/app/license.json" <= docker online
	licensePath := getLicensePath()

	if err := writeLicenseFile(licensePath, data); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to write license file"})
	}

	return c.JSON(fiber.Map{"message": "License activated", "license": license})
}

func GenerateActivationKeys(n int) error {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const keyLength = 16

	generateKey := func() string {
		b := make([]byte, keyLength)
		rand.Read(b)
		for i := 0; i < keyLength; i++ {
			b[i] = charset[int(b[i])%len(charset)]
		}
		return fmt.Sprintf("%s-%s-%s-%s", b[0:4], b[4:8], b[8:12], b[12:16])
	}

	var keys []model.ActivationKey
	existing := map[string]bool{}

	// เตรียม map จาก DB ที่มีอยู่ก่อนแล้ว
	var existingKeys []string
	db.Db.Model(&model.ActivationKey{}).Pluck("key", &existingKeys)
	for _, k := range existingKeys {
		existing[k] = true
	}

	count := 0
	for count < n {
		key := generateKey()
		if existing[key] {
			continue // ซ้ำใน DB แล้ว
		}

		keys = append(keys, model.ActivationKey{
			Key:       key,
			IsUsed:    false,
			CreatedAt: time.Now(),
		})
		existing[key] = true
		count++
	}

	if err := db.Db.Create(&keys).Error; err != nil {
		return fmt.Errorf("insert failed: %w", err)
	}

	fmt.Printf("✅ Generated %d new activation keys\n", n)
	return nil
}

func CheckLicenseStatus(c *fiber.Ctx) error {
	licensePath := getLicensePath()

	if !fileExists(licensePath) {
		machineID := generateMachineID()

		isActivated, activatedAt, err := checkLicenseWithServer(machineID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"valid": false, "error": "license server error"})
		}

		if err := createInitialLicenseFileWithStatus(licensePath, machineID, isActivated, activatedAt); err != nil {
			return c.Status(500).JSON(fiber.Map{"valid": false, "error": "failed to create license"})
		}

		return c.Status(200).JSON(fiber.Map{
			"valid":      isActivated,
			"message":    "license created",
			"machine_id": machineID,
		})
	}

	data, err := os.ReadFile(licensePath)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"valid": false, "error": "read error"})
	}

	var result struct {
		License struct {
			MachineID   string `json:"machine_id"`
			ActivatedAt string `json:"activated_at"`
			IsValid     bool   `json:"is_valid"`
		} `json:"license"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return c.Status(400).JSON(fiber.Map{"valid": false, "error": "invalid license format"})
	}

	if result.License.MachineID != generateMachineID() {
		return c.Status(400).JSON(fiber.Map{"valid": false, "error": "machine mismatch"})
	}

	if trustLocalLicense() && result.License.IsValid {
		return c.JSON(fiber.Map{"valid": true})
	}

	isActivated, activatedAt, err := checkLicenseWithServer(result.License.MachineID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"valid": false, "error": "license server error"})
	}

	if isActivated && (!result.License.IsValid || result.License.ActivatedAt == "") {
		if err := createInitialLicenseFileWithStatus(licensePath, result.License.MachineID, true, activatedAt); err != nil {
			return c.Status(500).JSON(fiber.Map{"valid": false, "error": "update local license failed"})
		}
	}

	if !result.License.IsValid {
		return c.Status(200).JSON(fiber.Map{
			"valid":      false,
			"message":    "license not yet activated",
			"machine_id": result.License.MachineID,
		})
	}

	return c.JSON(fiber.Map{"valid": true})
}

func checkLicenseWithServer(machineID string) (bool, string, error) {
	url := "http://165.232.161.93:3000/check-machine"

	reqBody := map[string]string{"machine_id": machineID}
	jsonBody, _ := json.Marshal(reqBody)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	var result struct {
		IsUsed      bool   `json:"is_used"`
		ActivatedAt string `json:"activated_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", err
	}

	return result.IsUsed, result.ActivatedAt, nil
}

func trustLocalLicense() bool {
	value := os.Getenv("LICENSE_TRUST_LOCAL")
	return value == "1" || value == "true" || value == "TRUE" || value == "yes" || value == "YES"
}

func createInitialLicenseFileWithStatus(path, machineID string, isValid bool, activatedAt string) error {
	if activatedAt == "" {
		activatedAt = time.Now().Format(time.RFC3339)
	}
	data := map[string]interface{}{
		"license": map[string]interface{}{
			"machine_id":   machineID,
			"is_valid":     isValid,
			"activated_at": activatedAt,
		},
	}
	file, _ := json.MarshalIndent(data, "", "  ")
	return writeLicenseFile(path, file)
}

// 🔧 helper: ตรวจไฟล์มีอยู่หรือไม่
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// 🔧 helper: สร้าง license.json เริ่มต้น
// func createInitialLicenseFile(path, machineID string) error {
// 	initial := fiber.Map{
// 		"license": fiber.Map{
// 			"machine_id":   machineID,
// 			"activated_at": nil,
// 			"is_valid":     false,
// 		},
// 	}
// 	data, err := json.MarshalIndent(initial, "", "  ")
// 	if err != nil {
// 		return err
// 	}
// 	return os.WriteFile(path, data, 0644)
// }

func generateMachineID() string {
	if machineID := os.Getenv("MACHINE_ID"); machineID != "" {
		return machineID
	}
	if seed := os.Getenv("MACHINE_ID_SEED"); seed != "" {
		return hashMachineID(seed)
	}

	hostname, _ := os.Hostname()
	mac := getPrimaryMAC()
	raw := hostname + "_" + mac

	return hashMachineID(raw)
}

func hashMachineID(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

func getPrimaryMAC() string {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if addr := iface.HardwareAddr.String(); addr != "" {
			return addr
		}
	}
	return "UNKNOWN"
}

func GetMachineID(c *fiber.Ctx) error {
	id := generateMachineID() // ใช้ฟังก์ชันเดียวกับฝั่ง activate
	return c.JSON(fiber.Map{"machine_id": id})
}
func getLicensePath() string {
	if path := os.Getenv("LICENSE_PATH"); path != "" {
		return path
	}
	ex, _ := os.Executable()
	dir := filepath.Dir(ex)
	return filepath.Join(dir, "license.json")
}
func EnsureLicenseFile() error {
	licensePath := getLicensePath()

	if _, err := os.Stat(licensePath); os.IsNotExist(err) {
		machineID := generateMachineID()

		initial := fiber.Map{
			"license": fiber.Map{
				"machine_id":   machineID,
				"activated_at": nil,
				"is_valid":     false,
			},
		}
		data, _ := json.MarshalIndent(initial, "", "  ")
		if err := writeLicenseFile(licensePath, data); err != nil {
			return fmt.Errorf("failed to write license.json: %v", err)
		}

		fmt.Println("✅ สร้าง license.json เริ่มต้นแล้ว")
	}
	return nil
}

func writeLicenseFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
