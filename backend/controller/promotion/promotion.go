package promotion

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

type promotionRequest struct {
	Name     string                       `json:"name"`
	StartAt  string                       `json:"start_at"`
	EndAt    string                       `json:"end_at"`
	Priority int                          `json:"priority"`
	IsActive *uint8                       `json:"is_active"`
	Tables   []promotionTablePriceRequest `json:"tables"`
}

type promotionTablePriceRequest struct {
	TableID       uint    `json:"table_id"`
	NormalPrice   float64 `json:"normal_price"`
	PracticePrice float64 `json:"practice_price"`
	IsActive      *uint8  `json:"is_active"`
}

func AnyData(c *fiber.Ctx) error {
	var promotions []model.Promotion
	err := db.Db.
		Where("deleted_at IS NULL").
		Order("start_at DESC, id DESC").
		Find(&promotions).Error
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load promotions"})
	}

	type row struct {
		model.Promotion
		Tables []model.PromotionTablePrice `json:"tables"`
	}
	result := make([]row, 0, len(promotions))
	for _, promotion := range promotions {
		var tables []model.PromotionTablePrice
		if err := db.Db.Where("promotion_id = ? AND deleted_at IS NULL", promotion.ID).Order("table_id ASC").Find(&tables).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load promotion tables"})
		}
		result = append(result, row{Promotion: promotion, Tables: tables})
	}
	return c.JSON(result)
}

func Store(c *fiber.Ctx) error {
	request, err := parsePromotionRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	err = db.Db.Transaction(func(tx *gorm.DB) error {
		promotion := model.Promotion{
			Name:     request.Name,
			StartAt:  request.startAt,
			EndAt:    request.endAt,
			Priority: request.Priority,
			IsActive: request.isActive,
		}
		if err := tx.Create(&promotion).Error; err != nil {
			return err
		}
		return replacePromotionTablePrices(tx, promotion.ID, request.Tables)
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create promotion"})
	}
	return c.JSON(fiber.Map{"message": "success"})
}

func Edit(c *fiber.Ctx) error {
	var promotion model.Promotion
	if err := db.Db.First(&promotion, c.Params("id")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "promotion not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load promotion"})
	}
	var tables []model.PromotionTablePrice
	if err := db.Db.Where("promotion_id = ? AND deleted_at IS NULL", promotion.ID).Order("table_id ASC").Find(&tables).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load promotion tables"})
	}
	return c.JSON(fiber.Map{"promotion": promotion, "tables": tables})
}

func Update(c *fiber.Ctx) error {
	request, err := parsePromotionRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	err = db.Db.Transaction(func(tx *gorm.DB) error {
		var promotion model.Promotion
		if err := tx.First(&promotion, c.Params("id")).Error; err != nil {
			return err
		}
		promotion.Name = request.Name
		promotion.StartAt = request.startAt
		promotion.EndAt = request.endAt
		promotion.Priority = request.Priority
		promotion.IsActive = request.isActive
		if err := tx.Save(&promotion).Error; err != nil {
			return err
		}
		return replacePromotionTablePrices(tx, promotion.ID, request.Tables)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "promotion not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update promotion"})
	}
	return c.JSON(fiber.Map{"message": "success"})
}

func Delete(c *fiber.Ctx) error {
	err := db.Db.Transaction(func(tx *gorm.DB) error {
		var promotion model.Promotion
		if err := tx.First(&promotion, c.Params("id")).Error; err != nil {
			return err
		}
		if err := tx.Where("promotion_id = ?", promotion.ID).Delete(&model.PromotionTablePrice{}).Error; err != nil {
			return err
		}
		return tx.Delete(&promotion).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "promotion not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete promotion"})
	}
	return c.JSON(fiber.Map{"message": "success"})
}

type parsedPromotionRequest struct {
	Name     string
	StartAt  string
	EndAt    string
	Priority int
	IsActive *uint8
	Tables   []promotionTablePriceRequest
	startAt  time.Time
	endAt    time.Time
	isActive uint8
}

func parsePromotionRequest(c *fiber.Ctx) (parsedPromotionRequest, error) {
	var request promotionRequest
	if err := c.BodyParser(&request); err != nil {
		return parsedPromotionRequest{}, errors.New("invalid request body")
	}
	if request.Name == "" || request.StartAt == "" || request.EndAt == "" {
		return parsedPromotionRequest{}, errors.New("name, start_at and end_at are required")
	}
	if len(request.Tables) == 0 {
		return parsedPromotionRequest{}, errors.New("at least one table price is required")
	}
	startAt, err := parseFlexibleTime(request.StartAt)
	if err != nil {
		return parsedPromotionRequest{}, errors.New("invalid start_at format")
	}
	endAt, err := parseFlexibleTime(request.EndAt)
	if err != nil {
		return parsedPromotionRequest{}, errors.New("invalid end_at format")
	}
	if !endAt.After(startAt) {
		return parsedPromotionRequest{}, errors.New("end_at must be after start_at")
	}

	isActive := uint8(1)
	if request.IsActive != nil {
		isActive = *request.IsActive
	}
	for _, table := range request.Tables {
		if table.TableID == 0 {
			return parsedPromotionRequest{}, errors.New("table_id is required")
		}
		if table.NormalPrice < 0 || table.PracticePrice < 0 {
			return parsedPromotionRequest{}, errors.New("promotion table prices must be non-negative")
		}
	}
	return parsedPromotionRequest{
		Name:     request.Name,
		StartAt:  request.StartAt,
		EndAt:    request.EndAt,
		Priority: request.Priority,
		IsActive: request.IsActive,
		Tables:   request.Tables,
		startAt:  startAt,
		endAt:    endAt,
		isActive: isActive,
	}, nil
}

func replacePromotionTablePrices(tx *gorm.DB, promotionID uint, tables []promotionTablePriceRequest) error {
	if err := tx.Where("promotion_id = ?", promotionID).Delete(&model.PromotionTablePrice{}).Error; err != nil {
		return err
	}
	for _, table := range tables {
		isActive := uint8(1)
		if table.IsActive != nil {
			isActive = *table.IsActive
		}
		record := model.PromotionTablePrice{
			PromotionID:   promotionID,
			TableID:       table.TableID,
			NormalPrice:   table.NormalPrice,
			PracticePrice: table.PracticePrice,
			IsActive:      isActive,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}

func parseFlexibleTime(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	var lastErr error
	location, _ := time.LoadLocation("Asia/Bangkok")
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, value, location)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}
