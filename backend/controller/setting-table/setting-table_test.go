package settingtable

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSettingTableTest(t *testing.T) (*fiber.App, uint, []model.SettingTable, model.SettingTable) {
	t.Helper()

	originalDB := db.Db
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "setting-table.db")+"?_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := testDB.AutoMigrate(&model.Division{}, &model.User{}, &model.SettingTable{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	division := model.Division{Code: "01", MaxDigits: "000000", Name: "Main", IsActive: 1}
	if err := testDB.Create(&division).Error; err != nil {
		t.Fatalf("create division: %v", err)
	}
	otherDivision := model.Division{Code: "02", MaxDigits: "000000", Name: "Other", IsActive: 1}
	if err := testDB.Create(&otherDivision).Error; err != nil {
		t.Fatalf("create other division: %v", err)
	}

	user := model.User{DivisionID: division.ID, Username: "tester", IsActive: 1}
	if err := testDB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	tables := []model.SettingTable{
		{DivisionID: division.ID, Name: "Table 1", Price: 100, Price2: 80, SortOrder: 1, IsActive: 1},
		{DivisionID: division.ID, Name: "Table 2", Price: 100, Price2: 80, SortOrder: 2, IsActive: 1},
	}
	for index := range tables {
		if err := testDB.Create(&tables[index]).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	otherTable := model.SettingTable{DivisionID: otherDivision.ID, Name: "Other Table", Price: 100, Price2: 80, SortOrder: 1, IsActive: 1}
	if err := testDB.Create(&otherTable).Error; err != nil {
		t.Fatalf("create other table: %v", err)
	}

	db.Db = testDB
	t.Cleanup(func() {
		sqlDB, _ := testDB.DB()
		_ = sqlDB.Close()
		db.Db = originalDB
	})

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", int(user.ID))
		return c.Next()
	})
	app.Post("/setting-tables/store", Store)
	app.Put("/setting-tables/reorder", Reorder)
	app.Put("/setting-tables/:id/update", Update)

	return app, division.ID, tables, otherTable
}

func TestReorderPersistsTableOrderForCurrentDivision(t *testing.T) {
	app, _, tables, _ := setupSettingTableTest(t)

	body := []byte(`{"items":[{"id":` + uintToJSON(tables[1].ID) + `,"sort_order":1},{"id":` + uintToJSON(tables[0].ID) + `,"sort_order":2}]}`)
	request := httptest.NewRequest("PUT", "/setting-tables/reorder", bytes.NewBuffer(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("reorder request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}

	var ordered []model.SettingTable
	if err := db.Db.Order("sort_order ASC").Find(&ordered, "division_id = ?", tables[0].DivisionID).Error; err != nil {
		t.Fatalf("load ordered tables: %v", err)
	}
	if ordered[0].ID != tables[1].ID || ordered[1].ID != tables[0].ID {
		t.Fatalf("order not persisted: got %d,%d want %d,%d", ordered[0].ID, ordered[1].ID, tables[1].ID, tables[0].ID)
	}
}

func TestReorderRejectsTablesOutsideCurrentDivision(t *testing.T) {
	app, _, tables, otherTable := setupSettingTableTest(t)

	body := []byte(`{"items":[{"id":` + uintToJSON(otherTable.ID) + `,"sort_order":1},{"id":` + uintToJSON(tables[0].ID) + `,"sort_order":2}]}`)
	request := httptest.NewRequest("PUT", "/setting-tables/reorder", bytes.NewBuffer(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("reorder request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.StatusCode)
	}

	var table model.SettingTable
	if err := db.Db.First(&table, tables[0].ID).Error; err != nil {
		t.Fatalf("load table: %v", err)
	}
	if table.SortOrder != 1 {
		t.Fatalf("current division table mutated after rejected reorder: got %d want 1", table.SortOrder)
	}
}

func TestStoreCanDisableRelayForFoodTable(t *testing.T) {
	app, divisionID, _, _ := setupSettingTableTest(t)

	body := []byte(`{"nameTable":"Food Table","typeTable":1,"price":0,"price2":0,"relayNumber":4,"address":"02","relayDisabled":true}`)
	request := httptest.NewRequest("POST", "/setting-tables/store", bytes.NewBuffer(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("store request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}

	var table model.SettingTable
	if err := db.Db.Where("division_id = ? AND name = ?", divisionID, "Food Table").First(&table).Error; err != nil {
		t.Fatalf("load stored table: %v", err)
	}
	if table.Relay != 0 || table.Address != "" {
		t.Fatalf("relay config was not disabled: relay=%d address=%q", table.Relay, table.Address)
	}
}

func TestUpdateCanDisableExistingRelay(t *testing.T) {
	app, _, tables, _ := setupSettingTableTest(t)

	body := []byte(`{"nameTable":"Table 1","typeTable":1,"price":100,"price2":80,"relay":3,"address":"01","noRelay":true}`)
	request := httptest.NewRequest("PUT", "/setting-tables/"+uintToJSON(tables[0].ID)+"/update", bytes.NewBuffer(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("update request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}

	var table model.SettingTable
	if err := db.Db.First(&table, tables[0].ID).Error; err != nil {
		t.Fatalf("load updated table: %v", err)
	}
	if table.Relay != 0 || table.Address != "" {
		t.Fatalf("relay config was not disabled: relay=%d address=%q", table.Relay, table.Address)
	}
}

func TestStoreRejectsInvalidRelayNumber(t *testing.T) {
	app, _, _, _ := setupSettingTableTest(t)

	body := []byte(`{"nameTable":"Bad Relay","typeTable":1,"price":0,"price2":0,"relayNumber":9,"address":"01"}`)
	request := httptest.NewRequest("POST", "/setting-tables/store", bytes.NewBuffer(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatalf("store request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.StatusCode)
	}
}

func uintToJSON(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
