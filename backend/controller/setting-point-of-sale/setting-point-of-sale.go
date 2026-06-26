package settingpointofsale

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

const (
	paymentQRModePromptPay       = "promptpay"
	paymentQRModeMerchantPayload = "merchant_payload"
)

func defaultSettingPointOfSale() model.SettingPointOfSale {
	return model.SettingPointOfSale{
		CalProcess:    30,
		PaymentQRMode: paymentQRModePromptPay,
		IsActive:      1,
	}
}

func getOrCreateSettingPointOfSale() (model.SettingPointOfSale, error) {
	var setting model.SettingPointOfSale
	if err := db.Db.First(&setting, 1).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			setting = defaultSettingPointOfSale()
			setting.ID = 1
			return setting, db.Db.Create(&setting).Error
		}
		return setting, err
	}

	return setting, nil
}

func normalizePromptPayID(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func validatePromptPayID(value string) (string, bool) {
	normalized := normalizePromptPayID(value)
	switch len(normalized) {
	case 10:
		return normalized, strings.HasPrefix(normalized, "0")
	case 13, 15:
		return normalized, true
	default:
		return normalized, false
	}
}

func normalizeMerchantQRPayload(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(value) {
		if char != ' ' && char != '\n' && char != '\r' && char != '\t' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func crc16CcittFalse(value string) string {
	crc := uint16(0xFFFF)
	for i := 0; i < len(value); i++ {
		crc ^= uint16(value[i]) << 8
		for bit := 0; bit < 8; bit++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return strings.ToUpper(fmt.Sprintf("%04X", crc))
}

func validateMerchantQRPayload(value string) (string, bool) {
	payload := normalizeMerchantQRPayload(value)
	if payload == "" || !strings.HasPrefix(payload, "000201") {
		return payload, false
	}
	crcIndex := strings.LastIndex(payload, "6304")
	if crcIndex < 0 || crcIndex+8 != len(payload) {
		return payload, false
	}
	expected := crc16CcittFalse(payload[:crcIndex+4])
	actual := payload[crcIndex+4:]
	return payload, expected == actual
}

func GetView(c *fiber.Ctx) error {
	setting, err := getOrCreateSettingPointOfSale()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Setting point of sale not found"})
	}

	return c.JSON(setting)
}

func GetSettingSystem(c *fiber.Ctx) error {
	id := c.Params("id")

	var setting model.SettingSystem
	if err := db.Db.First(&setting, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Setting system not found"})
	}

	return c.JSON(setting)
}

func SaveSettingPointOfSale(c *fiber.Ctx) error {
	setting, err := getOrCreateSettingPointOfSale()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Settings not found"})
	}

	billingInterval := c.FormValue("billing_interval")
	if billingInterval != "" {
		if val, err := strconv.Atoi(billingInterval); err == nil {
			setting.CalProcess = uint8(val)
		}
	}

	promptPayID := strings.TrimSpace(c.FormValue("prompt_pay_id"))
	paymentQRMode := strings.TrimSpace(c.FormValue("payment_qr_mode"))
	paymentQRPayload := strings.TrimSpace(c.FormValue("payment_qr_payload"))
	if paymentQRMode == "" {
		paymentQRMode = paymentQRModePromptPay
		if paymentQRPayload != "" {
			paymentQRMode = paymentQRModeMerchantPayload
		}
	}

	switch paymentQRMode {
	case paymentQRModeMerchantPayload:
		normalizedPayload, ok := validateMerchantQRPayload(paymentQRPayload)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid merchant QR payload"})
		}
		setting.PaymentQRMode = paymentQRModeMerchantPayload
		setting.PaymentQRPayload = normalizedPayload
		setting.PromptPayID = ""
	default:
		normalizedPromptPayID, ok := validatePromptPayID(promptPayID)
		if promptPayID != "" && !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid PromptPay ID"})
		}
		setting.PaymentQRMode = paymentQRModePromptPay
		setting.PaymentQRPayload = ""
		if promptPayID != "" {
			setting.PromptPayID = normalizedPromptPayID
		} else {
			setting.PromptPayID = ""
		}
	}

	if err := db.Db.Save(&setting).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update settings"})
	}

	fmt.Println("Settings updated successfully!")
	return c.JSON(setting)
}
