package inventory

import (
	"errors"
	"fmt"
	"sync"
	"time"

	model "github.com/rainza999/fiber-test/models"
	"gorm.io/gorm"
)

var (
	ErrInsufficientStock = errors.New("not enough stock")
	ErrInvalidQuantity   = errors.New("quantity must be greater than zero")
	mutationMu           sync.Mutex
)

const (
	MovementReceive    = "receive"
	MovementSale       = "sale"
	MovementReturn     = "return"
	MovementAdjustment = "adjustment"
)

func Migrate(database *gorm.DB) error {
	migrator := database.Migrator()
	if !migrator.HasTable(&model.ProductReceipt{}) {
		if err := database.AutoMigrate(&model.Supplier{}, &model.ProductReceipt{}, &model.ProductReceiptItem{}); err != nil {
			return err
		}
	} else if !migrator.HasTable(&model.ProductReceiptItem{}) {
		if err := database.AutoMigrate(&model.ProductReceiptItem{}); err != nil {
			return err
		}
	}
	if !migrator.HasTable(&model.StockEntry{}) {
		if err := database.AutoMigrate(&model.StockEntry{}); err != nil {
			return err
		}
	} else if !migrator.HasColumn(&model.StockEntry{}, "ReceiptItemKey") {
		if err := migrator.AddColumn(&model.StockEntry{}, "ReceiptItemKey"); err != nil {
			return err
		}
	}
	if err := database.AutoMigrate(&model.InventoryTransaction{}); err != nil {
		return err
	}

	statements := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_product_receipts_supplier_number_active
			ON product_receipts(supplier_id, receipt_number)
			WHERE deleted_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_product_receipt_items_receipt_product_active
			ON product_receipt_items(receipt_id, product_id)
			WHERE deleted_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_stock_entries_receipt_item_key
			ON stock_entries(receipt_item_key)
			WHERE receipt_item_key IS NOT NULL`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func WithTransaction(database *gorm.DB, fn func(tx *gorm.DB) error) error {
	mutationMu.Lock()
	defer mutationMu.Unlock()
	return database.Transaction(fn)
}

func ShouldManageStock(tx *gorm.DB, productID uint) (bool, error) {
	var result struct {
		IsStock uint8 `gorm:"column:is_stock"`
	}
	if err := tx.Table("products p").
		Select("COALESCE(c.is_stock, 0) as is_stock").
		Joins("LEFT JOIN categories c ON c.id = p.category_id").
		Where("p.id = ?", productID).
		Scan(&result).Error; err != nil {
		return false, err
	}
	return result.IsStock == 1, nil
}

func CreateReceiptStock(tx *gorm.DB, item model.ProductReceiptItem) (model.StockEntry, error) {
	if item.ID == 0 || item.ProductID == 0 || item.Quantity <= 0 || item.TotalPrice <= 0 {
		return model.StockEntry{}, errors.New("invalid receipt item")
	}
	var existingCount int64
	if err := tx.Model(&model.StockEntry{}).
		Where("product_receipt_item_id = ?", item.ID).
		Count(&existingCount).Error; err != nil {
		return model.StockEntry{}, err
	}
	if existingCount > 0 {
		return model.StockEntry{}, errors.New("receipt item already has stock")
	}
	key := fmt.Sprintf("receipt-item:%d", item.ID)
	entry := model.StockEntry{
		ProductID:            item.ProductID,
		StockLocationID:      1,
		Quantity:             item.Quantity,
		RemainingQty:         item.Quantity,
		CostPerUnit:          item.TotalPrice / float64(item.Quantity),
		ProductReceiptItemID: &item.ID,
		ReceiptItemKey:       &key,
		EntryDate:            time.Now(),
	}
	if err := tx.Create(&entry).Error; err != nil {
		return model.StockEntry{}, err
	}
	if err := recordMovement(tx, model.InventoryTransaction{
		ProductID:            item.ProductID,
		StockEntryID:         &entry.ID,
		ProductReceiptItemID: &item.ID,
		TransactionType:      MovementReceive,
		Quantity:             item.Quantity,
		BalanceChange:        item.Quantity,
		CostPerUnit:          entry.CostPerUnit,
		Notes:                "receipt finalized",
	}); err != nil {
		return model.StockEntry{}, err
	}
	return entry, nil
}

func DeductFIFO(tx *gorm.DB, productID, serviceID, visitationID uint, quantity int) (float64, error) {
	if quantity <= 0 {
		return 0, ErrInvalidQuantity
	}
	managed, err := ShouldManageStock(tx, productID)
	if err != nil || !managed {
		return 0, err
	}
	var available int
	if err := tx.Model(&model.StockEntry{}).
		Where("product_id = ? AND remaining_qty > 0", productID).
		Select("COALESCE(SUM(remaining_qty), 0)").
		Scan(&available).Error; err != nil {
		return 0, err
	}
	if available < quantity {
		return 0, ErrInsufficientStock
	}

	var entries []model.StockEntry
	if err := tx.Where("product_id = ? AND remaining_qty > 0", productID).
		Order("entry_date ASC, id ASC").
		Find(&entries).Error; err != nil {
		return 0, err
	}

	remaining := quantity
	totalCost := 0.0
	for _, entry := range entries {
		if remaining == 0 {
			break
		}
		used := min(remaining, entry.RemainingQty)
		result := tx.Model(&model.StockEntry{}).
			Where("id = ? AND remaining_qty >= ?", entry.ID, used).
			Update("remaining_qty", gorm.Expr("remaining_qty - ?", used))
		if result.Error != nil {
			return 0, result.Error
		}
		if result.RowsAffected != 1 {
			return 0, errors.New("stock changed while processing order")
		}
		totalCost += float64(used) * entry.CostPerUnit
		remaining -= used
		if err := recordServiceMovement(tx, productID, serviceID, visitationID, entry.ID, MovementSale, used, -used, entry.CostPerUnit, "POS order"); err != nil {
			return 0, err
		}
	}
	if remaining != 0 {
		return 0, ErrInsufficientStock
	}
	return totalCost, nil
}

func ReturnFIFO(tx *gorm.DB, productID, serviceID, visitationID uint, quantity int) (float64, error) {
	if quantity <= 0 {
		return 0, ErrInvalidQuantity
	}
	managed, err := ShouldManageStock(tx, productID)
	if err != nil || !managed {
		return 0, err
	}

	type allocation struct {
		StockEntryID uint
		Outstanding  int
		LastID       uint
	}
	var allocations []allocation
	if serviceID != 0 {
		if err := tx.Model(&model.InventoryTransaction{}).
			Select(`stock_entry_id,
				SUM(CASE WHEN transaction_type = ? THEN quantity ELSE -quantity END) AS outstanding,
				MAX(id) AS last_id`, MovementSale).
			Where("product_id = ? AND service_id = ? AND stock_entry_id IS NOT NULL AND transaction_type IN (?, ?)",
				productID, serviceID, MovementSale, MovementReturn).
			Group("stock_entry_id").
			Having("SUM(CASE WHEN transaction_type = ? THEN quantity ELSE -quantity END) > 0", MovementSale).
			Order("last_id DESC").
			Scan(&allocations).Error; err != nil {
			return 0, err
		}
	}

	remaining := quantity
	totalCost := 0.0
	for _, allocation := range allocations {
		if remaining == 0 {
			break
		}
		returned := min(remaining, allocation.Outstanding)
		cost, err := restoreEntry(tx, productID, serviceID, visitationID, allocation.StockEntryID, returned, "POS order return")
		if err != nil {
			return 0, err
		}
		totalCost += cost
		remaining -= returned
	}
	if remaining > 0 {
		cost, err := returnLegacyStock(tx, productID, serviceID, visitationID, remaining)
		if err != nil {
			return 0, err
		}
		totalCost += cost
	}
	return totalCost, nil
}

func AdjustStock(database *gorm.DB, productID uint, quantityChange int, costPerUnit float64, notes string) error {
	if quantityChange == 0 {
		return ErrInvalidQuantity
	}
	if notes == "" {
		return errors.New("adjustment notes are required")
	}
	if costPerUnit < 0 {
		return errors.New("cost per unit cannot be negative")
	}
	return WithTransaction(database, func(tx *gorm.DB) error {
		if quantityChange > 0 {
			entry := model.StockEntry{
				ProductID:       productID,
				StockLocationID: 1,
				Quantity:        quantityChange,
				RemainingQty:    quantityChange,
				CostPerUnit:     costPerUnit,
				EntryDate:       time.Now(),
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			return recordMovement(tx, model.InventoryTransaction{
				ProductID:       productID,
				StockEntryID:    &entry.ID,
				TransactionType: MovementAdjustment,
				Quantity:        quantityChange,
				BalanceChange:   quantityChange,
				CostPerUnit:     costPerUnit,
				Notes:           notes,
			})
		}
		return deductAdjustment(tx, productID, -quantityChange, notes)
	})
}

func deductAdjustment(tx *gorm.DB, productID uint, quantity int, notes string) error {
	var available int
	if err := tx.Model(&model.StockEntry{}).
		Where("product_id = ? AND remaining_qty > 0", productID).
		Select("COALESCE(SUM(remaining_qty), 0)").
		Scan(&available).Error; err != nil {
		return err
	}
	if available < quantity {
		return ErrInsufficientStock
	}
	var entries []model.StockEntry
	if err := tx.Where("product_id = ? AND remaining_qty > 0", productID).
		Order("entry_date ASC, id ASC").
		Find(&entries).Error; err != nil {
		return err
	}
	remaining := quantity
	for _, entry := range entries {
		if remaining == 0 {
			break
		}
		used := min(remaining, entry.RemainingQty)
		result := tx.Model(&model.StockEntry{}).
			Where("id = ? AND remaining_qty >= ?", entry.ID, used).
			Update("remaining_qty", gorm.Expr("remaining_qty - ?", used))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("stock changed while processing adjustment")
		}
		if err := recordMovement(tx, model.InventoryTransaction{
			ProductID:       productID,
			StockEntryID:    &entry.ID,
			TransactionType: MovementAdjustment,
			Quantity:        used,
			BalanceChange:   -used,
			CostPerUnit:     entry.CostPerUnit,
			Notes:           notes,
		}); err != nil {
			return err
		}
		remaining -= used
	}
	return nil
}

func returnLegacyStock(tx *gorm.DB, productID, serviceID, visitationID uint, quantity int) (float64, error) {
	var entries []model.StockEntry
	if err := tx.Where("product_id = ? AND remaining_qty < quantity", productID).
		Order("entry_date DESC, id DESC").
		Find(&entries).Error; err != nil {
		return 0, err
	}
	remaining := quantity
	totalCost := 0.0
	for _, entry := range entries {
		if remaining == 0 {
			break
		}
		returned := min(remaining, entry.Quantity-entry.RemainingQty)
		if returned <= 0 {
			continue
		}
		cost, err := restoreEntry(tx, productID, serviceID, visitationID, entry.ID, returned, "legacy POS order return")
		if err != nil {
			return 0, err
		}
		totalCost += cost
		remaining -= returned
	}
	if remaining != 0 {
		return 0, errors.New("unable to return all stock")
	}
	return totalCost, nil
}

func restoreEntry(tx *gorm.DB, productID, serviceID, visitationID, stockEntryID uint, quantity int, notes string) (float64, error) {
	var entry model.StockEntry
	if err := tx.First(&entry, stockEntryID).Error; err != nil {
		return 0, err
	}
	result := tx.Model(&model.StockEntry{}).
		Where("id = ? AND remaining_qty + ? <= quantity", entry.ID, quantity).
		Update("remaining_qty", gorm.Expr("remaining_qty + ?", quantity))
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, errors.New("stock changed while processing return")
	}
	if err := recordServiceMovement(tx, productID, serviceID, visitationID, entry.ID, MovementReturn, quantity, quantity, entry.CostPerUnit, notes); err != nil {
		return 0, err
	}
	return float64(quantity) * entry.CostPerUnit, nil
}

func recordServiceMovement(tx *gorm.DB, productID, serviceID, visitationID, stockEntryID uint, movement string, quantity, balanceChange int, costPerUnit float64, notes string) error {
	entryID := stockEntryID
	var serviceIDPtr, visitationIDPtr *uint
	if serviceID != 0 {
		serviceIDPtr = &serviceID
	}
	if visitationID != 0 {
		visitationIDPtr = &visitationID
	}
	return recordMovement(tx, model.InventoryTransaction{
		ProductID:       productID,
		StockEntryID:    &entryID,
		ServiceID:       serviceIDPtr,
		VisitationID:    visitationIDPtr,
		TransactionType: movement,
		Quantity:        quantity,
		BalanceChange:   balanceChange,
		CostPerUnit:     costPerUnit,
		Notes:           notes,
	})
}

func recordMovement(tx *gorm.DB, movement model.InventoryTransaction) error {
	return tx.Create(&movement).Error
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
