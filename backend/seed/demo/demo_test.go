package demo

import (
	"path/filepath"
	"testing"

	model "github.com/rainza999/fiber-test/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGenerateCreatesCleanUsableDemoDatabase(t *testing.T) {
	output := filepath.Join(t.TempDir(), "snooker.db")
	summary, err := Generate(Config{
		OutputPath:    output,
		AdminPassword: "test-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TableCount != 8 {
		t.Fatalf("table count = %d, want 8", summary.TableCount)
	}
	if summary.ProductCount != len(baselineProducts()) {
		t.Fatalf("product count = %d, want %d", summary.ProductCount, len(baselineProducts()))
	}

	database, err := gorm.Open(sqlite.Open(output), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	assertCount(t, database, &model.SettingTable{}, 8)
	assertCount(t, database, &model.Visitation{}, 0)
	assertCount(t, database, &model.Service{}, 0)
	assertCount(t, database, &model.ProductReceipt{}, 0)
	assertCount(t, database, &model.ProductReceiptItem{}, 0)
	assertCount(t, database, &model.StockEntry{}, int64(summary.OpeningStockItemCount))
	assertCount(t, database, &model.InventoryTransaction{}, int64(summary.OpeningStockItemCount))

	var settingSystem model.SettingSystem
	if err := database.First(&settingSystem).Error; err != nil {
		t.Fatal(err)
	}
	if settingSystem.FirstTime {
		t.Fatal("clean demo database must hide the one-time system settings menu")
	}

	var game model.Product
	if err := database.First(&game, 1).Error; err != nil {
		t.Fatal(err)
	}
	if !game.IsSnookerTime || game.Name != "ค่าเกม" {
		t.Fatalf("product 1 = %+v, want the snooker-time product", game)
	}

	var mismatchedTableCount int64
	if err := database.Model(&model.SettingTable{}).
		Where("price <> ? OR price2 <> ?", 150, 100).
		Count(&mismatchedTableCount).Error; err != nil {
		t.Fatal(err)
	}
	if mismatchedTableCount != 0 {
		t.Fatalf("found %d tables with mismatched rates", mismatchedTableCount)
	}

	var admin model.User
	if err := database.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte("test-password")); err != nil {
		t.Fatal("admin password was not hashed correctly")
	}

	var integrity string
	if err := database.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity check = %q, want ok", integrity)
	}

	if _, err := Generate(Config{OutputPath: output, AdminPassword: "test-password"}); err == nil {
		t.Fatal("expected a second generation without force to fail")
	}
}

func assertCount(t *testing.T, database *gorm.DB, modelValue interface{}, want int64) {
	t.Helper()
	var got int64
	if err := database.Model(modelValue).Count(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%T count = %d, want %d", modelValue, got, want)
	}
}
