package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"gorm.io/gorm"

	AuthController "github.com/rainza999/fiber-test/controller/auth"
	"github.com/rainza999/fiber-test/db"
	"github.com/rainza999/fiber-test/inventory"
	model "github.com/rainza999/fiber-test/models"

	// LicenseController "github.com/rainza999/fiber-test/controller/license"
	"github.com/rainza999/fiber-test/routes"

	// "github.com/dgrijalva/jwt-go"

	"os"
	"path/filepath"
)

type Config struct {
	AllowOrigins []string
}

//go:generate go run build.go -ldflags="-w -s"
func main() {

	// ตั้งค่า Timezone เป็น Asia/Bangkok (GMT+7)
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		log.Fatalf("Error loading location: %v", err)
	}
	time.Local = location // ตั้งค่า Local timezone เป็น Asia/Bangkok

	// ตัวอย่างการใช้งาน
	now := time.Now()
	log.Printf("Current time in GMT+7: %v", now)

	db.InitDB()
	db.Db.AutoMigrate(&model.SettingPointOfSale{})
	if err := inventory.Migrate(db.Db); err != nil {
		log.Fatalf("Failed to migrate inventory integrity schema: %v", err)
	}

	// db.Db.AutoMigrate(&model.ActivationKey{})
	// LicenseController.GenerateActivationKeys(1000)
	// db.Db.AutoMigrate(&model.SettingPointOfSale{})
	// MigrateStockEntries(db.Db)
	// UpdateStockEntriesWithReceiptItems(db.Db)
	//น่าจะแค่สองตัวนี้ครับ
	// MigrateStockEntrySoftDeleteAndRecreate(db.Db)
	// MigrateSupplierProductReceiptProductReceiptItem()
	// MigrateProductReceiptProductReceiptItem()
	// MigrateEmployeeAndUserData()
	// MigrateCompany(db.Db)
	// MigrateDivision(db.Db)
	// MigrateMenuAndPermissionData()
	// MigrateRole()
	// MigrateRoleHasPermission()
	// MigrateSettingTable(db.Db)
	// // MigrateVisitation()
	// MigrateProduct(db.Db)
	// MigrateCategory()
	// MigrateService(db.Db)
	// // MigrateStockLocation(db.Db)
	// MigrateStockEntry(db.Db)
	// MigrateSettingSystem(db.Db)
	// สร้าง client ของ Redis
	// AddStockForProduct(db.Db)
	// เพิ่ม stock entry สำหรับสินค้ารหัส product_id = 3
	// if err := AddStockForProduct3(db.Db); err != nil {
	// 	fmt.Println("Failed to add stock entry:", err)
	// } else {
	// 	fmt.Println("Stock entry added successfully")
	// }

	// client := redis.NewClient(&redis.Options{
	// 	Addr:     "redis:6379",
	// 	Password: "", // ใส่ password ถ้ามี
	// 	DB:       0,
	// })

	// ctx := context.Background()

	// ทำ caching

	// if err := Helper.GenerateCacheMenu(ctx, client); err != nil {
	// 	log.Fatal(err)
	// }

	// ดึงข้อมูล cache จาก Redis
	// cacheKey := "menus"
	// cacheData, err := client.Get(ctx, cacheKey).Result()
	// if err == redis.Nil {
	// 	fmt.Println("Cache not found")
	// } else if err != nil {
	// 	log.Fatal(err)
	// } else {
	// 	// แสดงผลลัพธ์ JSON ที่ได้จาก Redis
	// 	var prettyJSON bytes.Buffer
	// 	err := json.Indent(&prettyJSON, []byte(cacheData), "", "  ")
	// 	if err != nil {
	// 		log.Fatal("Failed to format JSON:", err)
	// 	}
	// 	fmt.Println("Cache Data:")
	// 	fmt.Println(prettyJSON.String())
	// }
	app := fiber.New()

	app.Use(cors.New(cors.Config{
		// AllowOrigins: "http://localhost:5173, http://127.0.0.1:5173",
		AllowOrigins: "http://localhost:5173,http://127.0.0.1:5173,http://128.199.223.79,http://165.22.242.231,http://127.0.0.1", // ไม่มี space หลัง comma
		// AllowOrigins: "http://localhost:5173,http://127.0.0.1:5173,http://127.0.0.1", // ไม่มี space หลัง comma

		// AllowOrigins:     "*",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: true,
	}))

	// เสิร์ฟไฟล์จากโฟลเดอร์ "uploads"
	// app.Static("/uploads", "./uploads")
	// dir := filepath.Join(os.Args[0], "..", "resources", "uploads") // 👈 คำนวณ path จริงหลัง build
	// fmt.Println(dir, " rainza 999")
	// app.Static("/uploads", dir)

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	rootDir := filepath.Dir(exePath) // ได้ path ถึงโฟลเดอร์ที่มี .exe อยู่

	// 👉 ถ้า uploads อยู่ภายใต้โฟลเดอร์เดียวกับ .exe
	uploadDir := filepath.Join(rootDir, "uploads")
	if configuredUploadDir := os.Getenv("UPLOAD_PATH"); configuredUploadDir != "" {
		uploadDir = configuredUploadDir
	}

	// 👉 ถ้า uploads อยู่ในโฟลเดอร์ resources ที่อยู่คู่กับ .exe
	// uploadDir := filepath.Join(rootDir, "resources", "uploads")

	fmt.Println("Upload Path:", uploadDir)

	app.Static("/uploads", uploadDir)

	// สร้าง *fiber.Ctx จาก context ของ Fiber
	// c := app.AcquireCtx(&fasthttp.RequestCtx{})
	// c := *&fiber.Ctx{}
	// routes.Setup(app)
	routes.Setup(app)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	log.Fatal(app.Listen(":" + port))
	log.Fatal("rain start!")
}

// func authRequired(c *fiber.Ctx) error {
// 	cookie := c.Cookies("jwt")
// 	fmt.Println(cookie)
// 	fmt.Println("why dont have cookie")
// 	jwtSecretKey := []byte("your-secret-key")
// 	token, err := jwt.ParseWithClaims(cookie, &jwt.StandardClaims{}, func(token *jwt.Token) (interface{}, error) {
// 		return jwtSecretKey, nil
// 	})

// 	if err != nil || !token.Valid {
// 		fmt.Println(cookie)
// 		fmt.Println("hello error")
// 		return c.SendStatus(fiber.StatusUnauthorized)
// 	}

// 	return c.Next()
// }

func MigrateStockEntries(db *gorm.DB) {
	type StockEntry struct {
		ID                   uint  `gorm:"primaryKey"`
		ProductReceiptItemID *uint // อนุญาตให้เป็น NULL ได้
	}

	// เพิ่ม column `product_receipt_item_id` ในตาราง stock_entries
	if err := db.AutoMigrate(&StockEntry{}); err != nil {
		fmt.Printf("Failed to migrate StockEntry: %v\n", err)
		return
	}

	fmt.Println("Migration for StockEntry completed successfully.")
}

func UpdateStockEntriesWithReceiptItems(db *gorm.DB) {
	type StockEntry struct {
		ID                   uint `gorm:"primaryKey"`
		Quantity             int
		CostPerUnit          float64
		DeletedAt            gorm.DeletedAt
		ProductReceiptItemID *uint `gorm:"column:product_receipt_item_id"`
	}

	type ProductReceiptItem struct {
		ID        uint `gorm:"primaryKey"`
		Quantity  int
		UnitPrice float64
	}

	// ดึงข้อมูล product_receipt_items ทั้งหมดมาเก็บใน map เพื่อ lookup
	var productReceiptItems []ProductReceiptItem
	if err := db.Find(&productReceiptItems).Error; err != nil {
		fmt.Printf("Failed to fetch product_receipt_items: %v\n", err)
		return
	}

	// แปลงเป็น map โดยใช้ (quantity, unit_price) เป็น key
	productReceiptItemMap := make(map[string]uint)
	for _, item := range productReceiptItems {
		key := fmt.Sprintf("%d-%.2f", item.Quantity, item.UnitPrice)
		productReceiptItemMap[key] = item.ID
	}

	// ดึง stock_entries ที่ยังไม่มี product_receipt_item_id และไม่ถูกลบ
	var stockEntries []StockEntry
	if err := db.Where("id >= ? AND product_receipt_item_id IS NULL AND deleted_at IS NULL", 404).
		Find(&stockEntries).Error; err != nil {
		fmt.Printf("Failed to fetch stock_entries: %v\n", err)
		return
	}

	// อัปเดต product_receipt_item_id
	for _, entry := range stockEntries {
		key := fmt.Sprintf("%d-%.2f", entry.Quantity, entry.CostPerUnit)
		if receiptItemID, exists := productReceiptItemMap[key]; exists {
			// อัปเดต stock_entries ให้เชื่อมกับ product_receipt_items
			if err := db.Model(&StockEntry{}).
				Where("id = ?", entry.ID).
				Update("product_receipt_item_id", receiptItemID).Error; err != nil {
				fmt.Printf("Failed to update stock_entry %d: %v\n", entry.ID, err)
			} else {
				fmt.Printf("Updated stock_entry %d with product_receipt_item_id %d\n", entry.ID, receiptItemID)
			}
		}
	}
}

// func MigrateStockEntrySoftDelete(db *gorm.DB) {
// 	type StockEntry struct {
// 		ID        uint `gorm:"primaryKey"`
// 		DeletedAt gorm.DeletedAt
// 	}

// 	if err := db.Model(&StockEntry{}).
// 		Where("id BETWEEN ? AND ?", 1, 397).
// 		Update("deleted_at", gorm.DeletedAt{Time: time.Now(), Valid: true}).Error; err != nil {
// 		fmt.Printf("Failed to update StockEntry records: %v\n", err)
// 		return
// 	}

//		fmt.Println("Soft delete applied successfully for StockEntry records with ID 1 to 397.")
//	}
// func MigrateStockEntrySoftDelete(db *gorm.DB) {
// 	// type StockEntry struct {
// 	// 	ID        uint `gorm:"primaryKey"`
// 	// 	ProductID uint
// 	// 	DeletedAt gorm.DeletedAt
// 	// }

// 	now := time.Now()

// 	// ใช้ raw SQL เพราะ GORM ไม่รองรับ join โดยตรงใน update
// 	if err := db.Exec(`
// 		UPDATE stock_entries
// 		SET deleted_at = ?
// 		WHERE id BETWEEN ? AND ?
// 		AND product_id NOT IN (
// 			SELECT id FROM products WHERE category_id = ?
// 		)
// 	`, now, 1, 403, 3).Error; err != nil {
// 		fmt.Printf("Failed to soft delete StockEntry records: %v\n", err)
// 		return
// 	}

// 	fmt.Println("Soft delete applied successfully for StockEntry records with ID 1 to 397 excluding category_id = 3.")
// }

func MigrateStockEntrySoftDeleteAndRecreate(db *gorm.DB) {
	now := time.Now()

	// Step 1: Soft delete ทุก stock_entry ที่ id <= 403
	if err := db.Exec(`
		UPDATE stock_entries
		SET deleted_at = ?
		WHERE id <= ?
	`, now, 403).Error; err != nil {
		fmt.Printf("❌ Failed to soft delete StockEntry records: %v\n", err)
		return
	}
	fmt.Println("✅ Soft delete completed for stock_entries with ID <= 403.")

	// Step 2: ดึง product_id จาก products ที่มี category_id = 3
	var productIDs []uint
	if err := db.Raw(`
		SELECT id FROM products WHERE category_id = ?
	`, 3).Scan(&productIDs).Error; err != nil {
		fmt.Printf("❌ Failed to fetch product_ids: %v\n", err)
		return
	}

	// Step 3: Insert รายการใหม่เข้า stock_entries
	type StockEntry struct {
		ProductID            uint
		StockLocationID      uint
		Quantity             int
		RemainingQty         int
		CostPerUnit          float64
		EntryDate            time.Time
		ProductReceiptItemID *uint
		CreatedAt            time.Time
		UpdatedAt            time.Time
	}

	var newEntries []StockEntry
	for _, productID := range productIDs {
		newEntries = append(newEntries, StockEntry{
			ProductID:            productID,
			StockLocationID:      1,
			Quantity:             999999,
			RemainingQty:         999999,
			CostPerUnit:          0,
			EntryDate:            now,
			ProductReceiptItemID: nil,
			CreatedAt:            now,
			UpdatedAt:            now,
		})
	}

	if len(newEntries) > 0 {
		if err := db.Table("stock_entries").Create(&newEntries).Error; err != nil {
			fmt.Printf("❌ Failed to create new StockEntry records: %v\n", err)
			return
		}
		fmt.Printf("✅ Created %d new StockEntry records for category_id = 3.\n", len(newEntries))
	} else {
		fmt.Println("⚠️ No product_ids found for category_id = 3.")
	}
}

func MigrateProductReceiptProductReceiptItem() {
	db.Db.Migrator().DropTable(&model.ProductReceipt{}, &model.ProductReceiptItem{})
	fmt.Println("Dropped existing ProductReceipt and ProductReceiptItem tables.")

	err := db.Db.AutoMigrate(&model.ProductReceipt{}, &model.ProductReceiptItem{})
	if err != nil {
		log.Fatalf("Failed to migrate tables: %v", err)
	}
	fmt.Println("ProductReceipt and ProductReceiptItem tables migrated successfully.")
	// db.Db.AutoMigrate(&model.ProductReceipt{}, &model.ProductReceiptItem{})
	// fmt.Println("ProductReceipt ProductReceiptItem table migrated successfully")
	//
}

func MigrateSupplierProductReceiptProductReceiptItem() {
	db.Db.Migrator().DropTable(&model.Supplier{}, &model.ProductReceipt{}, &model.ProductReceiptItem{})

	db.Db.AutoMigrate(&model.Supplier{}, &model.ProductReceipt{}, &model.ProductReceiptItem{})
	fmt.Println("Supplier ProductReceipt ProductReceiptItem table migrated successfully")
}
func MigrateRoleHasPermission() {
	db.Db.AutoMigrate(&model.RoleHasPermission{})
	fmt.Println("role_has_permissions table migrated successfully")
}

func MigrateVisitation() {
	db.Db.AutoMigrate(&model.Visitation{})
	fmt.Println("visitations table migrated successfully")
}

func MigrateService(db *gorm.DB) {
	db.AutoMigrate(&model.Service{})
	fmt.Println("service table migrated successfully")
}
func MigrateStockLocation(db *gorm.DB) {
	db.AutoMigrate(&model.StockLocation{})
	fmt.Println("StockLocation table migrated successfully")
}
func MigrateStockEntry(db *gorm.DB) {
	db.AutoMigrate(&model.StockEntry{})
	fmt.Println("StockEntry table migrated successfully")
}

func MigrateSettingSystem(db *gorm.DB) {
	db.AutoMigrate(&model.SettingSystem{})
	fmt.Println("SettingSystem table migrated successfully")
}

// ฟังก์ชันสำหรับการ migrate และเพิ่ม product สำหรับค่าโต๊ะสนุ๊ก
func MigrateCategory() {
	db.Db.AutoMigrate(&model.Category{})
	fmt.Println("categories table migrated successfully")
}

func MigrateProduct(db *gorm.DB) {
	db.AutoMigrate(&model.Product{})
	fmt.Println("products table migrated successfully")

	// ฟังก์ชันเพื่อเพิ่ม product ถ้ายังไม่มี
	addProductIfNotExists := func(name, description, unit string, price float64, isSnookerTime bool, isActive bool, categoryID uint) {
		var product model.Product
		if err := db.Where("name = ?", name).First(&product).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				product := model.Product{
					Name:          name,
					Description:   description,
					Price:         price,
					Unit:          unit,
					IsSnookerTime: isSnookerTime,
					IsActive:      isActive,
					CategoryID:    categoryID,
				}
				db.Create(&product)
				fmt.Printf("%s product created successfully\n", name)
			} else {
				fmt.Printf("Error checking for %s product: %v\n", name, err)
			}
		} else {
			fmt.Printf("%s product already exists\n", name)
		}
	}

	// เพิ่ม Snooker Table Time ถ้ายังไม่มี
	addProductIfNotExists(
		"ค่าเกม",
		"-",
		"hour",
		0,    // ราคาเริ่มต้นเป็น 0 เพราะราคาจะถูกคำนวณจาก SettingTable
		true, // ระบุว่าสินค้านี้เป็นค่าโต๊ะสนุ๊ก
		true,
		0,
	)

	addProductIfNotExists("น้ำเปล่า", "-", "ขวด", 15, false, true, 1)
	addProductIfNotExists("สปอนเซอร์", "-", "ขวด", 20, false, true, 1)
	addProductIfNotExists("น้ำโค้ก (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("น้ำแดง (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("น้ำเขียว (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("น้ำส้ม (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("น้ำแป๊บซี่ (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("น้ำชเวปส์มะนาว (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("น้ำสเเปลช", "-", "ขวด", 25, false, true, 1)
	addProductIfNotExists("ไวตามิล", "-", "ขวด", 25, false, true, 1)
	addProductIfNotExists("เบอร์ดี้ (แดง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("เนสกาแฟ (เขียว)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("C-vit", "-", "ขวด", 25, false, true, 1)
	addProductIfNotExists("M-150", "-", "ขวด", 20, false, true, 1)
	addProductIfNotExists("กระทิงแดง", "-", "ขวด", 20, false, true, 1)
	addProductIfNotExists("น้ำเลม่อนโซดา (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("กาแฟซอง 3 in 1", "-", "ซอง", 15, false, true, 1)
	addProductIfNotExists("น้ำสไปร์ท (กระป๋อง)", "-", "กระป๋อง", 25, false, true, 1)
	addProductIfNotExists("น้ำโออิชิ (เล็ก)", "-", "ขวด", 25, false, true, 1)
	addProductIfNotExists("ผ้าเย็น", "-", "ผืน", 20, false, true, 1)
	addProductIfNotExists("ชาลิปตัน (ขวด)", "-", "ขวด", 25, false, true, 1)
	addProductIfNotExists("ลิปโพ", "-", "ขวด", 20, false, true, 1)
	addProductIfNotExists("แบรนด์ ซุปไก่สกัด", "-", "ขวด", 70, false, true, 1)
	addProductIfNotExists("น้ำแข็ง", "-", "ถุง", 20, false, true, 1)
	addProductIfNotExists("โจ๊ก มาม่า", "-", "ถ้วย", 25, false, true, 4)
	addProductIfNotExists("ขนม 30 บาท", "-", "ห่อ", 30, false, true, 4)
	addProductIfNotExists("ขนม 10 บาท", "-", "ห่อ", 10, false, true, 4)
	addProductIfNotExists("เม็ดมะม่วง", "-", "ห่อ", 15, false, true, 4)
	addProductIfNotExists("ปลากระป๋อง, ปลากรอบ, แหนม", "-", "กระป๋อง", 35, false, true, 4)
	addProductIfNotExists("ไส้กรอก", "-", "ชิ้น", 50, false, true, 4)
	addProductIfNotExists("เบียร์สิงห์ (กระป๋อง)", "-", "กระป๋อง", 50, false, true, 2)
	addProductIfNotExists("เบียร์ลีโอ (กระป๋อง)", "-", "กระป๋อง", 50, false, true, 2)
	addProductIfNotExists("เบียร์สิงห์ (ขวด)", "-", "ขวด", 85, false, true, 2)
	addProductIfNotExists("เบียร์ลีโอ (ขวด)", "-", "ขวด", 80, false, true, 2)
	addProductIfNotExists("เบียร์ช้าง (ขวด)", "-", "ขวด", 80, false, true, 2)
	addProductIfNotExists("เบียร์ Heineken", "-", "ขวด", 100, false, true, 2)
	addProductIfNotExists("เหล้า หงษ์ทอง", "-", "ขวด", 350, false, true, 2)
	addProductIfNotExists("เหล้า แสงโสม", "-", "ขวด", 400, false, true, 2)
	addProductIfNotExists("เหล้า BLEND 285", "-", "ขวด", 350, false, true, 2)
	addProductIfNotExists("รีเจนซี่ (แบน)", "-", "แบน", 500, false, true, 2)
	addProductIfNotExists("โซดา", "-", "ขวด", 20, false, true, 2)
	addProductIfNotExists("บุหรี่", "-", "ซอง", 100, false, true, 2)
	addProductIfNotExists("หัวคิว LP Dream", "-", "ชิ้น", 600, false, true, 5)
	addProductIfNotExists("หัวคิว จอร์นปารีส", "-", "ชิ้น", 200, false, true, 5)
	addProductIfNotExists("หัวคิว Super Blue", "-", "ชิ้น", 150, false, true, 5)
	addProductIfNotExists("หัวคิว V Pro", "-", "ชิ้น", 150, false, true, 5)
	addProductIfNotExists("หัวคิว LP", "-", "ชิ้น", 180, false, true, 5)
	addProductIfNotExists("หัวคิว ฮันเตอร์", "-", "ชิ้น", 200, false, true, 5)
	addProductIfNotExists("หัวคิว มอนเตอร์", "-", "ชิ้น", 150, false, true, 5)
	addProductIfNotExists("หัวคิว เดอะซัน", "-", "ชิ้น", 150, false, true, 5)
	addProductIfNotExists("ชอล์กฝนหัวคิว ไทรแองเกิ้ลโปร", "-", "ชิ้น", 180, false, true, 5)
	addProductIfNotExists("ชอล์กฝนหัวคิว NIR", "-", "ชิ้น", 220, false, true, 5)
	addProductIfNotExists("ชอล์กฝนหัวคิว ธรรมดา", "-", "ชิ้น", 20, false, true, 5)
	addProductIfNotExists("ซองสวมหนังกันชื้นหัวคิว", "-", "ชิ้น", 80, false, true, 5)
	addProductIfNotExists("จุกยางแบบนุ่ม", "-", "ชิ้น", 70, false, true, 5)
	addProductIfNotExists("จุกยางทองเหลือง", "-", "ชิ้น", 100, false, true, 5)
	addProductIfNotExists("ฝอยขัด", "-", "ชิ้น", 20, false, true, 5)
	addProductIfNotExists("แม่เหล็กเหน็บชอล์ก", "-", "ชิ้น", 450, false, true, 5)
	addProductIfNotExists("กระเป๋าใส่ชอล์ก", "-", "ชิ้น", 150, false, true, 5)
	addProductIfNotExists("โวลเค่นคลีน", "-", "ชิ้น", 260, false, true, 5)
	addProductIfNotExists("โวลเค่นแว็กซ์", "-", "ชิ้น", 260, false, true, 5)
	addProductIfNotExists("โวลเค่นฟาส", "-", "ชิ้น", 300, false, true, 5)
	addProductIfNotExists("หัวคิว SPS", "-", "ชิ้น", 180, false, true, 5)
	addProductIfNotExists("ค่าเปิดขวด", "-", "ชิ้น", 100, false, true, 5)
	addProductIfNotExists("ลูกเต๋า", "-", "ชิ้น", 200, false, true, 5)
	addProductIfNotExists("น้ำยา A304", "-", "ชิ้น", 180, false, true, 5)
	addProductIfNotExists("สเปรย์ ซุปเปอร์สมูท", "-", "ชิ้น", 190, false, true, 5)
	addProductIfNotExists("ค่าจัดแข่ง", "-", "ชิ้น", 1500, false, true, 5)
	addProductIfNotExists("ชอล์กฝนหัวคิว ATOM", "-", "ชิ้น", 750, false, true, 5)
	addProductIfNotExists("หัวคิว บลูอัด", "-", "ชิ้น", 140, false, true, 5)
	addProductIfNotExists("ข้าวกะเพราเนื้อ", "-", "จาน", 75, false, true, 3)
	addProductIfNotExists("ข้าวกะเพราหมู/ไก่", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("หมูทอด", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ยำทะเล", "-", "จาน", 100, false, true, 3)
	addProductIfNotExists("ข้าวราดแกง", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ไข่ดาว", "-", "ฟอง", 20, false, true, 3)
	addProductIfNotExists("เฟรนช์ฟรายส์", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ไก่คาราเกะ", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("นักเก็ตไก่", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ผัดมาม่าหมู", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ข้าวหมู-ไก่ทอดกระเทียม", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("จาน ถ้วย", "-", "ชุด", 5, false, true, 3)
	addProductIfNotExists("ข้าวกะเพราไก่คาราเกะ", "-", "จาน", 95, false, true, 3)
	addProductIfNotExists("ไข่เจียว", "-", "ฟอง", 25, false, true, 3)
	addProductIfNotExists("ข้าวไข่ดาว/ไข่เจียว", "-", "จาน", 45, false, true, 3)
	addProductIfNotExists("ข้าวไข่เจียว หมูสับ", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("พิเศษ หมู", "-", "จาน", 10, false, true, 3)
	addProductIfNotExists("พิเศษ เนื้อ", "-", "จาน", 15, false, true, 3)
	addProductIfNotExists("ข้าวเนื้อทอดกระเทียม", "-", "จาน", 75, false, true, 3)
	addProductIfNotExists("ผัดมาม่าเนื้อ", "-", "จาน", 75, false, true, 3)
	addProductIfNotExists("ข้าวผัดพริกแกงเนื้อ", "-", "จาน", 75, false, true, 3)
	addProductIfNotExists("ต้มยำไก่", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ยำไข่ดาว", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("แหนม", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("หมูแดดเดียว", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ยำวุ้นเส้นหมูสับ", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ยำหมูยอ", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ลาบหมู", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ไข่ตุ๋น", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ต้มชุปเปอร์ตีนไก่", "-", "จาน", 100, false, true, 3)
	addProductIfNotExists("ต้มยำกุ้งทะเล หมึก กุ้ง", "-", "จาน", 100, false, true, 3)
	addProductIfNotExists("สุกี้หมู ไก่", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("สุกี้ทะเล", "-", "จาน", 75, false, true, 3)
	addProductIfNotExists("ไข่เจียวแหนม", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("หมูมะนาว", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("ผัดมาม่าทะเล", "-", "จาน", 75, false, true, 3)
	addProductIfNotExists("พิเศษทะเล", "-", "จาน", 20, false, true, 3)
	addProductIfNotExists("ข้าวเปล่า", "-", "จาน", 20, false, true, 3)
	addProductIfNotExists("ไข่เจียวหมูสับ", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ข้าวไข่เจียวหมูสับ", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ข้าวผัดหมู", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ข้าวผัดทะเล", "-", "จาน", 85, false, true, 3)
	addProductIfNotExists("ข้าวผัดเนื้อ", "-", "จาน", 75, false, true, 3)
	addProductIfNotExists("ข้าวราดพริกแกงหมู ไก่", "-", "จาน", 65, false, true, 3)
	addProductIfNotExists("ข้าวราดพริกแกงรวมทะเล", "-", "จาน", 85, false, true, 3)
	addProductIfNotExists("พริกแกงหมู ไก่", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("พริกแกงเนื้อ", "-", "จาน", 90, false, true, 3)
	addProductIfNotExists("พริกแกงทะเล", "-", "จาน", 100, false, true, 3)
	addProductIfNotExists("กะเพราเนื้อ", "-", "จาน", 90, false, true, 3)
	addProductIfNotExists("กะเพราทะเล", "-", "จาน", 100, false, true, 3)
	addProductIfNotExists("หม ไก่ ทอดกระเทียม", "-", "จาน", 80, false, true, 3)
	addProductIfNotExists("เนื้อทอดกระเทียม", "-", "จาน", 90, false, true, 3)
	addProductIfNotExists("ทะเลทอดกระเทียม", "-", "จาน", 100, false, true, 3)
	addProductIfNotExists("ข้าวทะเลทอดกระเทียม", "-", "จาน", 85, false, true, 3)
	addProductIfNotExists("ข้าวไข่เจียวทะเล", "-", "จาน", 85, false, true, 3)
	addProductIfNotExists("ไข่เจียวทะเล", "-", "จาน", 85, false, true, 3)
	addProductIfNotExists("แหนมซี่โครงหมู", "-", "จาน", 120, false, true, 3)
	addProductIfNotExists("ข้าวกะเพราทะเล", "-", "จาน", 85, false, true, 3)
	addProductIfNotExists("ข้าวต้มหมูสับ", "-", "ชาม", 65, false, true, 3)
	addProductIfNotExists("ข้าวต้มทะเล - เนื้อ", "-", "ชาม", 75, false, true, 3)
	addProductIfNotExists("ข้าวผัดแหนม", "-", "จาน", 85, false, true, 3)
	addProductIfNotExists("ข้าวไข่เจียวหมูสับพิเศษ", "-", "จาน", 85, false, true, 3)

	// // เพิ่ม Food ถ้ายังไม่มี
	// addProductIfNotExists(
	// 	"อาหารและครื่องดื่ม",
	// 	"This product represents food&item items",
	// 	"item",
	// 	0,     // ราคาเริ่มต้นเป็น 0, สามารถเปลี่ยนแปลงได้
	// 	false, // ไม่ใช่ค่าโต๊ะสนุ๊ก
	// )

	// เพิ่ม Drink ถ้ายังไม่มี
	// addProductIfNotExists(
	// 	"Drink",
	// 	"This product represents beverage items",
	// 	"item",
	// 	0,     // ราคาเริ่มต้นเป็น 0, สามารถเปลี่ยนแปลงได้
	// 	false, // ไม่ใช่ค่าโต๊ะสนุ๊ก
	// )
}

func MigrateSettingTable(db *gorm.DB) {
	db.AutoMigrate(&model.SettingTable{})
	fmt.Println("setting_tables table migrated successfully")
}

func MigrateCompany(db *gorm.DB) {
	db.AutoMigrate(&model.Company{})
	fmt.Println("companies table migrated successfully")

	companies := []struct {
		Company model.Company
	}{
		{
			Company: model.Company{
				Name: "ห้างหุ้นส่วนจำกัด โอเอ็ม สนุ๊กเกอร์ คลับ", Status: "active",
			},
		}, {
			Company: model.Company{
				Name: "ช. ช้าง สนุ๊กเกอร์คลับ", Status: "active",
			},
		},
	}

	for _, item := range companies {
		db.Create(&item.Company)
	}
	fmt.Println("Companies data migrated successfully")
}

func MigrateDivision(db *gorm.DB) {
	db.AutoMigrate(&model.Division{})
	fmt.Println("divisions table migrated successfully")

	divisions := []struct {
		Division model.Division
	}{
		{
			Division: model.Division{
				Code: "01", MaxDigits: "000000", Name: "สาขา1", ShortName: "SOM1", Address: "420หมู่1 ต.บางบ่อ อ.บางบ่อ จ.สมุทรปราการ 10560", Tel: "0815936532", Line: "rain..2", Display: 1, Status: "active",
			},
		},
	}

	for _, item := range divisions {
		db.Create(&item.Division)
	}
	fmt.Println("Divisions data migrated successfully")
}

func MigrateRole() {
	db.Db.AutoMigrate(&model.Role{})
	fmt.Println("roles table migrated successfully")
}

func MigrateMenuAndPermissionData() {
	db.Db.AutoMigrate(&model.Menu{}, &model.Permission{})

	menusAndPermissions := []struct {
		Menu        model.Menu
		Permissions []model.Permission
	}{
		{
			Menu: model.Menu{
				Name: "การคิดเงิน", Route: "/point-of-sale", Level: 0, HasSub: 0, Order: 1, Icon: "AttachMoneyIcon", IsActive: 1,
			},
			Permissions: []model.Permission{
				{
					Name: "point-of-sale-access", Title: "เข้าถึง",
				},
				{
					Name: "point-of-sale-create", Title: "เพิ่มข้อมูล",
				},
				{
					Name: "point-of-sale-edit", Title: "แก้ไขข้อมูล",
				},
				{
					Name: "point-of-sale-delete", Title: "ลบข้อมูล",
				},
			},
		},
		{
			Menu: model.Menu{
				Name: "จัดการข้อมูล", Route: "#", Level: 0, HasSub: 1, Order: 2, IsActive: 1,
			},
		},
		{
			Menu: model.Menu{
				Name: "ข้อมูลโต๊ะสนุ๊ก", Route: "/setting-table", Level: 1, Relation: 2, HasSub: 0, Order: 1, Icon: "AppsIcon", IsActive: 1,
			},
			Permissions: []model.Permission{
				{
					Name: "setting-table-access", Title: "เข้าถึง",
				},
				{
					Name: "setting-table-create", Title: "เพิ่มข้อมูล",
				},
				{
					Name: "setting-table-edit", Title: "แก้ไขข้อมูล",
				},
				{
					Name: "setting-table-delete", Title: "ลบข้อมูล",
				},
			},
		},
		{
			Menu: model.Menu{
				Name: "ข้อมูลเวลา", Route: "/setting-timer", Level: 1, Relation: 2, HasSub: 0, Order: 2, Icon: "AlarmAddIcon", IsActive: 1,
			},
			Permissions: []model.Permission{
				{
					Name: "setting-timer-access", Title: "เข้าถึง",
				},
				{
					Name: "setting-timer-create", Title: "เพิ่มข้อมูล",
				},
				{
					Name: "setting-timer-edit", Title: "แก้ไขข้อมูล",
				},
				{
					Name: "setting-timer-delete", Title: "ลบข้อมูล",
				},
			},
		},
		{
			Menu: model.Menu{
				Name: "จัดการผู้ใช้งาน", Route: "#", Level: 0, HasSub: 1, Order: 3, IsActive: 1,
			},
		},
		{
			Menu: model.Menu{
				Name: "รายชื่อผู้ใช้งาน", Route: "/users", Level: 1, Relation: 5, HasSub: 0, Order: 1, Icon: "ManageAccountsIcon", IsActive: 1,
			},
			Permissions: []model.Permission{
				{
					Name: "users-access", Title: "เข้าถึง",
				},
				{
					Name: "users-create", Title: "เพิ่มข้อมูล",
				},
				{
					Name: "users-edit", Title: "แก้ไขข้อมูล",
				},
				{
					Name: "users-delete", Title: "ลบข้อมูล",
				},
			},
		},
		{
			Menu: model.Menu{
				Name: "สิทธิ์การใช้งาน", Route: "/roles", Level: 1, Relation: 5, HasSub: 0, Order: 2, Icon: "SecurityIcon", IsActive: 1,
			},
			Permissions: []model.Permission{
				{
					Name: "roles-access", Title: "เข้าถึง",
				},
				{
					Name: "roles-create", Title: "เพิ่มข้อมูล",
				},
				{
					Name: "roles-edit", Title: "แก้ไขข้อมูล",
				},
				{
					Name: "roles-delete", Title: "ลบข้อมูล",
				},
			},
		},
	}

	for _, item := range menusAndPermissions {
		// Hash the password before creating the user

		// Create employee
		db.Db.Create(&item.Menu)

		for i := range item.Permissions {
			item.Permissions[i].MenuID = item.Menu.ID
			db.Db.Create(&item.Permissions[i])
		}
	}
	fmt.Println("Menu And Permission data migrated successfully")
}

func MigrateEmployeeAndUserData() {
	// Migrate employee data
	db.Db.AutoMigrate(&model.Employee{}, &model.User{})

	// ใช้ time.Format เพื่อกำหนดรูปแบบของวันที่
	// Create sample employees and users
	employeesAndUsers := []struct {
		Employee model.Employee
		User     model.User
	}{
		{
			Employee: model.Employee{
				FirstName: "John", LastName: "Cena", NickName: "Johnny", Email: "john.cena@example.com", Telephone: "0123456789", DateOfJoining: time.Now().UTC().Truncate(24 * time.Hour), Status: "active",
			},
			User: model.User{
				Username: "john", Password: "hashed_password_1", DivisionID: 1, RoleID: 1,
			},
		},
		{
			Employee: model.Employee{
				FirstName: "Patchanok", LastName: "Arayasujin", NickName: "Rain", Email: "rainstep1607@gmail.com", Telephone: "0815936532", DateOfJoining: time.Now().UTC().Truncate(24 * time.Hour), Status: "active",
			},
			User: model.User{
				Username: "rain", Password: "1234", DivisionID: 1, RoleID: 1,
			},
		},
		{
			Employee: model.Employee{
				FirstName: "Dutchun", LastName: "Lastname", NickName: "Dutch", Email: "dutch@xample.com", Telephone: "0999999999", DateOfJoining: time.Now().UTC().Truncate(24 * time.Hour), Status: "active",
			},
			User: model.User{
				Username: "dutch", Password: "1234", DivisionID: 1, RoleID: 1,
			},
		},
		// Add more employee and user data as needed
	}

	for _, item := range employeesAndUsers {
		// Hash the password before creating the user
		hashedPassword, err := AuthController.HashPassword(item.User.Password)
		if err != nil {
			log.Fatal(err)
		}
		item.User.Password = hashedPassword

		// Create employee
		db.Db.Create(&item.Employee)

		// Create associated user
		item.User.EmployeeID = item.Employee.ID
		db.Db.Create(&item.User)
	}

	fmt.Println("Employee and User data migrated successfully")
}

// func MigrateEmployeeData() {
// 	// Migrate employee data
// 	db.Db.AutoMigrate(&model.Employee{})

// 	// Create sample employees
// 	employees := []model.Employee{
// 		{FirstName: "John", LastName: "Cena", NickName: "Johnny", Email: "john.cena@example.com", Telephone: "0123456789", Status: "Active", IsActive: 1},
// 		{FirstName: "Patchanok", LastName: "Arayasujin", NickName: "Rain", Email: "rainstep1607@gmail.com", Telephone: "0815936532", Status: "Active", IsActive: 1},
// 		{FirstName: "Dutchun", LastName: "Lastname", NickName: "Dutch", Email: "dutch@xample.com", Telephone: "0999999999", Status: "Active", IsActive: 1},
// 		// Add more employee data as needed
// 	}

// 	for _, employee := range employees {
// 		db.Db.Create(&employee)
// 	}

// 	fmt.Println("Employee data migrated successfully")
// }

// func MigrateUserData() {
// 	// Migrate user data
// 	db.Db.AutoMigrate(&model.User{})

// 	// Create sample users
// 	users := []model.User{
// 		{Username: "john", Password: "hashed_password_1", IsActive: 1, EmployeeID: 1},
// 		{Username: "rain", Password: "1234", IsActive: 1, EmployeeID: 2},
// 		{Username: "dutch", Password: "1234", IsActive: 1, EmployeeID: 3},
// 		// Add more user data as needed
// 	}

// 	for _, user := range users {
// 		// Hash the password before creating the user
// 		hashedPassword, err := AuthController.HashPassword(user.Password)
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 		user.Password = hashedPassword

// 		db.Db.Create(&user)
// 	}

// 	fmt.Println("User data migrated successfully")
// }
// func MigrateUserData() {
// 	// Migrate user data
// 	db.Db.AutoMigrate(&model.User{})

// 	// Create sample users
// 	users := []model.User{
// 		{Username: "user1", Password: "hashed_password_1", Fullname: "User One"},
// 		{Username: "user2", Password: "hashed_password_2", Fullname: "User Two"},
// 		{Username: "user3", Password: "hashed_password_3", Fullname: "User Three"},
// 		{Username: "dutchun", Password: "1234", Fullname: "Dutchun"},
// 		{Username: "rain", Password: "1234", Fullname: "Rain"},
// 	}

// 	for _, user := range users {
// 		// Hash the password before creating the user
// 		hashedPassword, err := AuthController.HashPassword(user.Password)
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 		user.Password = hashedPassword

// 		db.Db.Create(&user)
// 	}

// 	fmt.Println("User data migrated successfully")
// }

// func AddStockForProduct3(db *gorm.DB) error {
// 	// กำหนดจำนวนรอบและจำนวนสินค้าในแต่ละรอบ
// 	rounds := 5
// 	quantityPerRound := 10
// 	costPerUnit := 10.0 // ต้นทุนต่อหน่วย

// 	for i := 0; i < rounds; i++ {
// 		stockEntry := model.StockEntry{
// 			ProductID:       3, // product_id = 3
// 			StockLocationID: 1, // Main Stock หรือ stock ที่ต้องการ
// 			Quantity:        quantityPerRound,
// 			CostPerUnit:     costPerUnit,
// 		}

// 		result := db.Create(&stockEntry)
// 		if result.Error != nil {
// 			return result.Error
// 		}

// 		fmt.Printf("Added %d units in round %d\n", quantityPerRound, i+1)
// 	}

//		return nil
//	}
func AddStockForProduct(db *gorm.DB) error {
	// ดึงข้อมูล product ทั้งหมด ยกเว้น id 1 และ id 2
	var products []model.Product
	if err := db.Where("id NOT IN ?", []int{1}).Find(&products).Error; err != nil {
		return fmt.Errorf("error fetching products: %v", err)
	}

	// สร้าง stock entries สำหรับแต่ละ product ที่ดึงมา
	for _, product := range products {
		stockEntry := model.StockEntry{
			ProductID:       product.ID,        // ดึง id ของ product นั้นๆ
			StockLocationID: 1,                 // Main Stock หรือ stock ที่ต้องการ (ปรับได้ตามที่ต้องการ)
			Quantity:        99999,             // จำนวนสินค้าที่จะเพิ่ม
			CostPerUnit:     product.Price - 5, // ราคาเดิมจาก product แล้วลบออก 5 บาท
			RemainingQty:    99999,
		}

		if stockEntry.CostPerUnit < 0 {
			stockEntry.CostPerUnit = 0 // ป้องกันไม่ให้ CostPerUnit ติดลบ
		}

		if err := db.Create(&stockEntry).Error; err != nil {
			fmt.Printf("Error creating stock entry for product %s: %v\n", product.Name, err)
		} else {
			fmt.Printf("Stock entry created for product %s with cost per unit: %.2f\n", product.Name, stockEntry.CostPerUnit)
		}
	}

	return nil
}
