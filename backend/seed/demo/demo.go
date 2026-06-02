package demo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	OutputPath         string
	AdminUsername      string
	AdminPassword      string
	CloseTablePassword string
	EditReportPassword string
	CompanyName        string
	DivisionName       string
	Force              bool
}

type Summary struct {
	OutputPath            string
	TableCount            int
	CategoryCount         int
	ProductCount          int
	SupplierCount         int
	OpeningStockItemCount int
}

type productSeed struct {
	Name          string
	CategoryID    uint
	Price         float64
	Unit          string
	OpeningQty    int
	OpeningCost   float64
	IsSnookerTime bool
}

type menuSeed struct {
	ID          uint
	Name        string
	Route       string
	Level       uint8
	Relation    uint
	HasSub      uint
	Order       uint8
	Icon        string
	Permissions []permissionSeed
}

type permissionSeed struct {
	Name  string
	Title string
}

func Generate(config Config) (Summary, error) {
	config = withDefaults(config)
	if err := validateConfig(config); err != nil {
		return Summary{}, err
	}

	outputPath, err := filepath.Abs(config.OutputPath)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output path: %w", err)
	}
	if err := prepareOutput(outputPath, config.Force); err != nil {
		return Summary{}, err
	}

	complete := false
	defer func() {
		if !complete {
			removeSQLiteFiles(outputPath)
		}
	}()

	database, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(outputPath)+"?_busy_timeout=5000&_foreign_keys=1"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return Summary{}, fmt.Errorf("open sqlite database: %w", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		return Summary{}, fmt.Errorf("get sqlite connection: %w", err)
	}
	defer sqlDB.Close()

	if err := migrate(database); err != nil {
		return Summary{}, fmt.Errorf("migrate demo database: %w", err)
	}

	summary := Summary{OutputPath: outputPath}
	if err := database.Transaction(func(tx *gorm.DB) error {
		return seedBaseData(tx, config, &summary)
	}); err != nil {
		return Summary{}, fmt.Errorf("seed demo database: %w", err)
	}

	for _, product := range baselineProducts() {
		if product.OpeningQty <= 0 {
			continue
		}
		var saved model.Product
		if err := database.Where("name = ?", product.Name).First(&saved).Error; err != nil {
			return Summary{}, fmt.Errorf("load product %q for opening stock: %w", product.Name, err)
		}
		if err := inventory.AdjustStock(database, saved.ID, product.OpeningQty, product.OpeningCost, "demo opening stock"); err != nil {
			return Summary{}, fmt.Errorf("create opening stock for %q: %w", product.Name, err)
		}
		summary.OpeningStockItemCount++
	}

	var integrity string
	if err := database.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		return Summary{}, fmt.Errorf("run sqlite integrity check: %w", err)
	}
	if integrity != "ok" {
		return Summary{}, fmt.Errorf("sqlite integrity check returned %q", integrity)
	}

	complete = true
	return summary, nil
}

func withDefaults(config Config) Config {
	if strings.TrimSpace(config.AdminUsername) == "" {
		config.AdminUsername = "admin"
	}
	if strings.TrimSpace(config.CloseTablePassword) == "" {
		config.CloseTablePassword = config.AdminPassword
	}
	if strings.TrimSpace(config.EditReportPassword) == "" {
		config.EditReportPassword = config.CloseTablePassword
	}
	if strings.TrimSpace(config.CompanyName) == "" {
		config.CompanyName = "YS Snooker Demo"
	}
	if strings.TrimSpace(config.DivisionName) == "" {
		config.DivisionName = "สาขาทดลอง"
	}
	return config
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.OutputPath) == "" {
		return errors.New("output path is required")
	}
	if strings.TrimSpace(config.AdminPassword) == "" {
		return errors.New("admin password is required")
	}
	return nil
}

func prepareOutput(path string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		if !force {
			return fmt.Errorf("output already exists: %s (use --force to replace it)", path)
		}
		removeSQLiteFiles(path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output path: %w", err)
	}
	return nil
}

func removeSQLiteFiles(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + "-shm")
	_ = os.Remove(path + "-wal")
}

func migrate(database *gorm.DB) error {
	if err := database.AutoMigrate(
		&model.ActivationKey{},
		&model.Company{},
		&model.Division{},
		&model.SettingSystem{},
		&model.SettingPointOfSale{},
		&model.SettingTable{},
		&model.Category{},
		&model.Product{},
		&model.Employee{},
		&model.Role{},
		&model.User{},
		&model.Menu{},
		&model.Permission{},
		&model.RoleHasPermission{},
		&model.Supplier{},
		&model.Location{},
		&model.Unit{},
		&model.ProductStock{},
		&model.StockLocation{},
		&model.Visitation{},
		&model.Service{},
		&model.ProductReceipt{},
		&model.ProductReceiptItem{},
		&model.StockEntry{},
		&model.InventoryTransaction{},
	); err != nil {
		return err
	}
	return inventory.Migrate(database)
}

func seedBaseData(tx *gorm.DB, config Config, summary *Summary) error {
	company := model.Company{Name: config.CompanyName, Status: "active", IsActive: 1}
	if err := tx.Create(&company).Error; err != nil {
		return err
	}

	division := model.Division{
		Code:        "01",
		MaxDigits:   "000000",
		Name:        config.DivisionName,
		ShortName:   "DEMO",
		Address:     "สำหรับสาธิตระบบ",
		OpeningDate: time.Now().Truncate(24 * time.Hour),
		Display:     1,
		Status:      "active",
		IsActive:    1,
	}
	if err := tx.Create(&division).Error; err != nil {
		return err
	}

	settingSystem := model.SettingSystem{
		LogoPath:      "uploads/logo.jpg",
		LogoLoginPath: "uploads/LOGO_Final_Stoke2.png",
		FirstTime:     false,
		IsActive:      1,
	}
	if err := settingSystem.SetCloseTablePassword(config.CloseTablePassword); err != nil {
		return err
	}
	if err := settingSystem.SetEditReportPassword(config.EditReportPassword); err != nil {
		return err
	}
	if err := tx.Create(&settingSystem).Error; err != nil {
		return err
	}
	if err := tx.Create(&model.SettingPointOfSale{CalProcess: 30, IsActive: 1}).Error; err != nil {
		return err
	}

	for index := 1; index <= 8; index++ {
		table := model.SettingTable{
			DivisionID: division.ID,
			Code:       fmt.Sprintf("T%02d", index),
			Name:       fmt.Sprintf("โต๊ะที่ %d", index),
			Price:      150,
			Price2:     100,
			Ma:         1,
			Type:       1,
			Status:     "active",
			IsActive:   1,
			Relay:      uint8(index),
			Address:    "01",
		}
		if err := tx.Create(&table).Error; err != nil {
			return err
		}
		summary.TableCount++
	}

	categories := []model.Category{
		{ID: 1, Name: "เครื่องดื่ม", Status: "active", IsActive: 1, IsStock: 1},
		{ID: 2, Name: "สุรา/บุหรี่", Status: "active", IsActive: 1, IsStock: 1},
		{ID: 3, Name: "อาหาร", Status: "active", IsActive: 1, IsStock: 0},
		{ID: 4, Name: "ขนม/ของทานเล่น", Status: "active", IsActive: 1, IsStock: 1},
		{ID: 5, Name: "อุปกรณ์สนุ๊กเกอร์", Status: "active", IsActive: 1, IsStock: 1},
	}
	if err := tx.Create(&categories).Error; err != nil {
		return err
	}
	summary.CategoryCount = len(categories)

	for _, item := range baselineProducts() {
		product := model.Product{
			CategoryID:    item.CategoryID,
			Name:          item.Name,
			Description:   "-",
			Price:         item.Price,
			Unit:          item.Unit,
			IsSnookerTime: item.IsSnookerTime,
			Status:        "active",
			IsActive:      true,
		}
		if err := tx.Create(&product).Error; err != nil {
			return err
		}
		summary.ProductCount++
	}

	suppliers := []model.Supplier{
		{Name: "ผู้จำหน่ายเครื่องดื่ม", Contact: "-", Address: "-", IsActive: true},
		{Name: "ผู้จำหน่ายอาหารและของทานเล่น", Contact: "-", Address: "-", IsActive: true},
		{Name: "ผู้จำหน่ายอุปกรณ์สนุ๊กเกอร์", Contact: "-", Address: "-", IsActive: true},
	}
	if err := tx.Create(&suppliers).Error; err != nil {
		return err
	}
	summary.SupplierCount = len(suppliers)

	if err := tx.Create(&model.StockLocation{ID: 1, Name: "คลังหลัก", IsPrimary: true}).Error; err != nil {
		return err
	}
	if err := tx.Create(&model.Location{ID: 1, Name: "คลังหลัก", Address: "-"}).Error; err != nil {
		return err
	}
	for _, unitName := range []string{"ขวด", "กระป๋อง", "จาน", "ถ้วย", "ถุง", "ซอง", "ชิ้น", "ผืน"} {
		if err := tx.Create(&model.Unit{Name: unitName, ConversionRate: 1}).Error; err != nil {
			return err
		}
	}

	role := model.Role{Name: "SYSTEM", IsActive: 1}
	if err := tx.Create(&role).Error; err != nil {
		return err
	}

	for _, item := range baselineMenus() {
		menu := model.Menu{
			Model:    gorm.Model{ID: item.ID},
			Name:     item.Name,
			Route:    item.Route,
			Level:    item.Level,
			Relation: item.Relation,
			HasSub:   item.HasSub,
			Order:    item.Order,
			Icon:     item.Icon,
			IsActive: 1,
		}
		if err := tx.Create(&menu).Error; err != nil {
			return err
		}
		for _, permissionItem := range item.Permissions {
			permission := model.Permission{
				Name:     permissionItem.Name,
				Title:    permissionItem.Title,
				MenuID:   menu.ID,
				IsActive: 1,
			}
			if err := tx.Create(&permission).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.RoleHasPermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
				return err
			}
		}
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(config.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	employee := model.Employee{
		FirstName:     "Demo",
		LastName:      "Admin",
		NickName:      "Admin",
		DateOfJoining: time.Now().Truncate(24 * time.Hour),
		Status:        "active",
		IsActive:      1,
	}
	if err := tx.Create(&employee).Error; err != nil {
		return err
	}
	return tx.Create(&model.User{
		EmployeeID: employee.ID,
		DivisionID: division.ID,
		RoleID:     role.ID,
		Username:   config.AdminUsername,
		Password:   string(passwordHash),
		IsActive:   1,
	}).Error
}

func baselineProducts() []productSeed {
	return []productSeed{
		{Name: "ค่าเกม", Unit: "hour", IsSnookerTime: true},
		{Name: "น้ำเปล่า", CategoryID: 1, Price: 15, Unit: "ขวด", OpeningQty: 48, OpeningCost: 8},
		{Name: "น้ำอัดลม", CategoryID: 1, Price: 25, Unit: "กระป๋อง", OpeningQty: 36, OpeningCost: 15},
		{Name: "โซดา", CategoryID: 1, Price: 20, Unit: "ขวด", OpeningQty: 24, OpeningCost: 10},
		{Name: "M-150", CategoryID: 1, Price: 20, Unit: "ขวด", OpeningQty: 24, OpeningCost: 12},
		{Name: "กาแฟกระป๋อง", CategoryID: 1, Price: 30, Unit: "กระป๋อง", OpeningQty: 24, OpeningCost: 18},
		{Name: "เบียร์ลีโอ", CategoryID: 2, Price: 80, Unit: "ขวด", OpeningQty: 24, OpeningCost: 55},
		{Name: "เบียร์ช้าง", CategoryID: 2, Price: 80, Unit: "ขวด", OpeningQty: 24, OpeningCost: 55},
		{Name: "ข้าวกะเพราหมู/ไก่", CategoryID: 3, Price: 65, Unit: "จาน"},
		{Name: "ข้าวผัดหมู", CategoryID: 3, Price: 65, Unit: "จาน"},
		{Name: "เฟรนช์ฟรายส์", CategoryID: 3, Price: 65, Unit: "จาน"},
		{Name: "นักเก็ตไก่", CategoryID: 3, Price: 65, Unit: "จาน"},
		{Name: "ไข่ดาว", CategoryID: 3, Price: 15, Unit: "ฟอง"},
		{Name: "มาม่าถ้วย", CategoryID: 4, Price: 25, Unit: "ถ้วย", OpeningQty: 12, OpeningCost: 15},
		{Name: "ขนมขบเคี้ยว", CategoryID: 4, Price: 20, Unit: "ถุง", OpeningQty: 20, OpeningCost: 12},
		{Name: "ถั่วทอด", CategoryID: 4, Price: 20, Unit: "ถุง", OpeningQty: 20, OpeningCost: 12},
		{Name: "ผ้าเย็น", CategoryID: 4, Price: 20, Unit: "ผืน", OpeningQty: 24, OpeningCost: 10},
		{Name: "ชอล์กฝนหัวคิว", CategoryID: 5, Price: 20, Unit: "ชิ้น", OpeningQty: 12, OpeningCost: 10},
		{Name: "หัวคิวมาตรฐาน", CategoryID: 5, Price: 150, Unit: "ชิ้น", OpeningQty: 5, OpeningCost: 90},
	}
}

func baselineMenus() []menuSeed {
	crud := func(prefix string) []permissionSeed {
		return []permissionSeed{
			{Name: prefix + "-access", Title: "เข้าถึง"},
			{Name: prefix + "-create", Title: "เพิ่มข้อมูล"},
			{Name: prefix + "-edit", Title: "แก้ไขข้อมูล"},
			{Name: prefix + "-delete", Title: "ลบข้อมูล"},
		}
	}

	return []menuSeed{
		{ID: 1, Name: "การคิดเงิน", Route: "/point-of-sales", Order: 1, Icon: "AttachMoneyIcon", Permissions: append(crud("point-of-sale"), permissionSeed{Name: "point-of-sale-lighting-access", Title: "ใช้งานปุ่มทดสอบไฟ"})},
		{ID: 2, Name: "จัดการข้อมูล", Route: "#", HasSub: 1, Order: 2},
		{ID: 3, Name: "ข้อมูลโต๊ะสนุ๊ก", Route: "/setting-tables", Level: 1, Relation: 2, Order: 1, Icon: "AppsIcon", Permissions: crud("setting-table")},
		{ID: 5, Name: "จัดการผู้ใช้งาน", Route: "#", HasSub: 1, Order: 4},
		{ID: 6, Name: "รายชื่อผู้ใช้งาน", Route: "/users", Level: 1, Relation: 5, Order: 1, Icon: "ManageAccountsIcon", Permissions: crud("users")},
		{ID: 7, Name: "สิทธิ์การใช้งาน", Route: "/roles", Level: 1, Relation: 5, Order: 2, Icon: "SecurityIcon", Permissions: crud("roles")},
		{ID: 8, Name: "ข้อมูลสาขา", Route: "/divisions", Level: 1, Relation: 2, Order: 2, Icon: "AppsIcon", Permissions: crud("divisions")},
		{ID: 10, Name: "รายงาน", Route: "#", HasSub: 1, Order: 3},
		{ID: 11, Name: "รายงานยอดขาย", Route: "/sale-reports", Level: 1, Relation: 10, Order: 1, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "sale-reports-access", Title: "เข้าถึง"}, {Name: "sale-reports-edit", Title: "แก้ไขข้อมูล"}}},
		{ID: 12, Name: "รายงานแจกแจงสินค้า", Route: "/sale-product-reports", Level: 1, Relation: 10, Order: 2, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "sale-product-reports-access", Title: "เข้าถึง"}}},
		{ID: 13, Name: "ข้อมูลหมวดหมู่", Route: "/categories", Level: 1, Relation: 2, Order: 3, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "categories-access", Title: "เข้าถึง"}, {Name: "categories-create-access", Title: "เพิ่มข้อมูล"}, {Name: "categories-edit-access", Title: "แก้ไขข้อมูล"}, {Name: "categories-delete", Title: "ลบข้อมูล"}}},
		{ID: 14, Name: "ข้อมูลสินค้า", Route: "/products", Level: 1, Relation: 2, Order: 4, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "products-access", Title: "เข้าถึง"}, {Name: "products-create-access", Title: "เพิ่มข้อมูล"}, {Name: "products-edit-access", Title: "แก้ไขข้อมูล"}, {Name: "products-delete-access", Title: "ลบข้อมูล"}}},
		{ID: 15, Name: "ตั้งค่าระบบ", Route: "/setting-system", Order: 5, Icon: "AppIcon", Permissions: []permissionSeed{{Name: "setting-system-access", Title: "เข้าถึง"}}},
		{ID: 16, Name: "ข้อมูลผู้จำหน่าย", Route: "/suppliers", Level: 1, Relation: 2, Order: 5, Icon: "AppsIcon", Permissions: crud("suppliers")},
		{ID: 17, Name: "ข้อมูลรับเข้าสินค้า", Route: "/product-receipts", Level: 1, Relation: 2, Order: 6, Icon: "AppsIcon", Permissions: crud("product-receipts")},
		{ID: 18, Name: "รายงานรับเข้าสินค้า", Route: "/product-receipt-reports", Level: 1, Relation: 10, Order: 3, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "product-receipt-reports-access", Title: "เข้าถึง"}, {Name: "product-receipt-reports-edit", Title: "แก้ไขข้อมูล"}, {Name: "product-receipt-reports-delete", Title: "ลบข้อมูล"}}},
		{ID: 19, Name: "รายงานสินค้าคงเหลือ", Route: "/product-stocks", Level: 1, Relation: 10, Order: 4, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "product-stocks-access", Title: "เข้าถึง"}, {Name: "product-stocks-edit", Title: "แก้ไขข้อมูล"}, {Name: "product-stocks-delete", Title: "ลบข้อมูล"}}},
		{ID: 20, Name: "รายงานความเคลื่อนไหวของสินค้า", Route: "/product-transactions", Level: 1, Relation: 10, Order: 5, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "product-transactions-access", Title: "เข้าถึง"}, {Name: "product-transactions-edit", Title: "แก้ไขข้อมูล"}, {Name: "product-transactions-delete", Title: "ลบข้อมูล"}}},
		{ID: 21, Name: "ตั้งค่าการคิดเงิน", Route: "/setting-point-of-sales", Level: 1, Relation: 2, Order: 7, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "setting-point-of-sale-access", Title: "เข้าถึง"}}},
		{ID: 22, Name: "รายงานยอดขายปิดบิล", Route: "/sale-report-closes", Level: 1, Relation: 10, Order: 6, Icon: "AppsIcon", Permissions: []permissionSeed{{Name: "sale-report-closes-access", Title: "เข้าถึง"}}},
	}
}
