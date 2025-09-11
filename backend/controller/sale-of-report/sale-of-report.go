package saleofreport

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rainza999/fiber-test/db"
)

// Struct สำหรับแสดงข้อมูลรายงานการขาย (แยกออกจากโครงสร้างในฐานข้อมูล)
type VisitationReport struct {
	TableName  string  `json:"table_name"`
	BillNumber string  `json:"bill_number"`
	StartDate  string  `json:"start_date"`
	StartTime  string  `json:"start_time"`
	EndDate    string  `json:"end_date"`
	EndTime    string  `json:"end_time"`
	TotalBill  float64 `json:"total_bill"`
	Uuid       string  `json:"uuid"`
}

// ฟังก์ชันดึงข้อมูลรายงานยอดขายรายวัน
func GetDailySalesReport(c *fiber.Ctx) error {
	startDate, endDate := c.Query("start_date"), c.Query("end_date")

	// ตรวจสอบวันที่
	if err := validateDateRange(startDate, endDate); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// แปลง endDate ให้เป็นสิ้นสุดวัน (23:59:59)
	endDate = endDate + " 23:59:59"

	// Query ข้อมูลจากฐานข้อมูล
	var visitations []struct {
		BillCode     string    `json:"bill_code"`
		StartTime    time.Time `json:"start_time"`
		EndTime      time.Time `json:"end_time"`
		NetPrice     float64   `json:"net_price"`
		TotalCost    float64   `json:"total_cost"`
		TableID      uint      `json:"table_id"`
		PaidAmount   float64   `json:"paid_amount"`
		ChangeAmount float64   `json:"change_amount"`
		Uuid         string    `json:"uuid"`
	}
	err := db.Db.Raw(`SELECT bill_code, start_time, end_time, net_price, total_cost, paid_amount, change_amount, table_id, uuid
		FROM visitations WHERE (start_time BETWEEN ? AND ? ) and (is_paid = 1) and deleted_at is null and is_active = 1`, startDate, endDate).Scan(&visitations).Error
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// แปลงข้อมูล start_time ให้เป็นแยก "ปี-เดือน-วัน" และ "เวลา HH:MM"
	var reportData []VisitationReport
	for _, v := range visitations {
		reportData = append(reportData, VisitationReport{
			TableName:  getTableName(v.TableID), // ฟังก์ชันสำหรับดึงชื่อโต๊ะ
			BillNumber: v.BillCode,
			StartDate:  v.StartTime.Format("2006-01-02"), // แปลงเป็น "ปี-เดือน-วัน"
			StartTime:  v.StartTime.Format("15:04"),      // แปลงเป็น "HH:MM"
			EndDate:    v.EndTime.Format("2006-01-02"),   // แปลงเป็น "ปี-เดือน-วัน"
			EndTime:    v.EndTime.Format("15:04"),        // แปลงเป็น "HH:MM"
			TotalBill:  v.NetPrice,
			Uuid:       v.Uuid,
		})
	}

	// ตรวจสอบว่ามีข้อมูลหรือไม่ ถ้าไม่มีให้ส่ง response ที่เหมาะสมกลับไป
	if len(reportData) == 0 {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"visitations": nil})
	}

	return c.JSON(fiber.Map{"visitations": reportData})
}

// ฟังก์ชันสำหรับดึงรายละเอียดรายงานการขาย (detail)
func GetDailySalesReportDetail(c *fiber.Ctx) error {
	// ดึง uuid จาก URL parameter
	uuid := c.Params("uuid")

	// Query ข้อมูลของบิลจาก table visitations
	var visitation struct {
		ID           uint    `json:"id"` // visitation_id เพื่อเอาไปใช้ค้นใน services
		BillCode     string  `json:"bill_code"`
		TableID      uint    `json:"table_id"`
		TableName    string  `json:"table_name"`
		StartTime    string  `json:"start_time"`
		EndTime      string  `json:"end_time"`
		NetPrice     float64 `json:"net_price"`
		TotalCost    float64 `json:"total_cost"`
		PaidAmount   float64 `json:"paid_amount"`
		ChangeAmount float64 `json:"change_amount"`
		TableType    uint    `json:"table_type"`
		Price        float64 `json:"price"`
		Price2       float64 `json:"price2"`
	}
	// Query ข้อมูลจาก table visitations โดยใช้ uuid
	err := db.Db.Raw(`SELECT visitations.id, bill_code, 
    table_id, start_time, end_time, net_price, total_cost, paid_amount, change_amount, table_type, setting_tables.name as table_name,
    setting_tables.price as price, 
    setting_tables.price2 as price2


		FROM visitations left join setting_tables on visitations.table_id = setting_tables.id WHERE uuid = ?`, uuid).Scan(&visitation).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "ไม่พบข้อมูลรายละเอียดของรายงาน",
		})
	}
	// Query ข้อมูลจาก table services โดย join กับ products
	var serviceDetails []struct {
		ProductID    uint    `json:"product_id"`    // รหัสสินค้า
		ProductName  string  `json:"product_name"`  // ชื่อสินค้า (จาก products table)
		SellQuantity float64 `json:"sell_quantity"` // จำนวน
		TotalCost    float64 `json:"total_cost"`    // ราคา
		NetPrice     float64 `json:"net_price"`     // ราคาสุทธิ
	}

	// Query ข้อมูลจาก services ที่เชื่อมกับ visitation_id และ products
	err = db.Db.Raw(`
		SELECT 
			services.product_id,
			products.name AS product_name,
			services.sell_quantity,
			services.total_cost,
			services.net_price
		FROM services
		LEFT JOIN products ON services.product_id = products.id
		WHERE services.visitation_id = ? and services.deleted_at is null and services.status = 'paid'`, visitation.ID).Scan(&serviceDetails).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "ไม่พบข้อมูลใน services",
		})
	}

	// ส่งข้อมูลกลับไปในรูปแบบ JSON
	return c.JSON(fiber.Map{
		"visitation":     visitation,
		"serviceDetails": serviceDetails,
	})
}

// ฟังก์ชันตรวจสอบรูปแบบวันที่
func validateDateRange(startDate, endDate string) error {
	layout := "2006-01-02"
	_, errStart := time.Parse(layout, startDate)
	_, errEnd := time.Parse(layout, endDate)
	if errStart != nil || errEnd != nil {
		return errors.New("รูปแบบวันที่ไม่ถูกต้อง")
	}
	return nil
}

// ฟังก์ชันสมมุติสำหรับดึงชื่อโต๊ะจาก TableID
func getTableName(tableID uint) string {
	var tableName string
	db.Db.Raw("SELECT name FROM setting_tables WHERE id = ?", tableID).Scan(&tableName)
	return tableName
}

func GetMonthlySalesReport(c *fiber.Ctx) error {
	// ดึงเดือนที่เลือกจาก query parameter
	selectedMonth := c.Query("month") // รูปแบบ "YYYY-MM"

	// Query ดึงข้อมูลยอดขายตามเดือน
	var report []struct {
		Date              string  `json:"date"`
		GameFee           float64 `json:"game_fee"`
		FoodFee           float64 `json:"food_fee"`
		DrinkFee          float64 `json:"drink_fee"`
		SportEquipmentFee float64 `json:"sport_equipment_fee"`
		TotalFee          float64 `json:"total_fee"`
	}

	query := `
    SELECT 
        DATE(datetime(visitations.start_time, '+7 hours')) AS date,
		SUM(CASE WHEN services.product_id = 1 THEN services.net_price ELSE 0 END) AS game_fee,
		SUM(CASE WHEN categories.id = 3 THEN services.net_price ELSE 0 END) AS food_fee,
        SUM(CASE WHEN categories.id = 1 THEN services.net_price ELSE 0 END) AS drink_fee,
		SUM(CASE WHEN categories.id = 4 THEN services.net_price ELSE 0 END) AS cat_4,
    	SUM(CASE WHEN categories.id = 2 THEN services.net_price ELSE 0 END) AS cat_2,
		SUM(CASE WHEN categories.id in (5,6,7,8) THEN services.net_price ELSE 0 END) AS cat_5678,
        SUM(services.net_price) AS total_fee
    FROM visitations
    JOIN services ON visitations.id = services.visitation_id
	join products on products.id = services.product_id
	left join categories on categories.id = products.category_id
    WHERE services.status = 'paid' AND visitations.start_time BETWEEN ? AND ?
    GROUP BY DATE(datetime(visitations.start_time, '+7 hours'))
    ORDER BY DATE(datetime(visitations.start_time, '+7 hours'))
`

	// แปลง selectedMonth ที่เป็น "YYYY-MM" เพื่อกำหนดช่วงวันที่
	startOfMonth := selectedMonth + "-01"
	endOfMonth := selectedMonth + "-31"

	// ส่งค่า startOfMonth และ endOfMonth แทน DATE_FORMAT
	if err := db.Db.Raw(query, startOfMonth, endOfMonth).Scan(&report).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "ไม่สามารถดึงข้อมูลรายงานรายเดือนได้",
		})
	}

	// return c.JSON(report)
	return c.JSON(fiber.Map{"report": report})
}

func GetMonthlySaleProductReport(c *fiber.Ctx) error {
	selectedMonth := c.Query("month") // รูปแบบ "YYYY-MM"

	// แปลง selectedMonth ให้เป็น time.Time และหาวันที่สิ้นสุดของเดือน
	startOfMonth, err := time.Parse("2006-01", selectedMonth)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "รูปแบบวันที่ไม่ถูกต้อง",
		})
	}

	// หาวันที่สิ้นสุดของเดือน
	endOfMonth := startOfMonth.AddDate(0, 1, -1) // เพิ่ม 1 เดือนแล้วลบ 1 วัน

	log.Printf("Start of month: %s", startOfMonth.Format("2006-01-02"))
	log.Printf("End of month: %s", endOfMonth.Format("2006-01-02"))
	var report []struct {
		TypeName     string  `json:"type_name"`      // ประเภท เช่น ค่าเกมส์, อาหาร
		ProductName  string  `json:"product_name"`   // ชื่อสินค้า หรือชื่อโต๊ะ (กรณี product_id = 1)
		PricePerUnit float64 `json:"price_per_unit"` // ราคาต่อหน่วย
		Qty          float64 `json:"qty"`            // จำนวน
		NetPrice     float64 `json:"net_price"`      // ราคาสุทธิ
	}

	query := `
	SELECT 
		CASE 
			WHEN s.product_id = 1 THEN 'ค่าเกมส์'
			ELSE c.name
		END AS type_name,
		CASE 
			WHEN s.product_id = 1 THEN 
				st.name || ' (' || 
				CASE 
					WHEN v.table_type = 0 THEN 'ปกติ'
					ELSE 'ซ้อม'
				END || ')'
			ELSE p.name
		END AS product_name,
		p.price as price_per_unit,
		SUM(s.sell_quantity) AS qty,
		SUM(s.net_price) AS net_price
	FROM 
		services s
	LEFT JOIN 
		visitations v ON s.visitation_id = v.id
	LEFT JOIN 
		setting_tables st ON v.table_id = st.id
	LEFT JOIN 
		products p ON s.product_id = p.id
	LEFT JOIN 
		categories c ON p.category_id = c.id
	WHERE 
		s.status = 'paid'
		AND s.deleted_at IS NULL 
		AND v.start_time BETWEEN ? AND ?
	GROUP BY 
		CASE 
			WHEN s.product_id = 1 THEN v.table_id || '_' || v.table_type
			ELSE s.product_id
		END,
		CASE 
			WHEN s.product_id = 1 THEN v.table_type
			ELSE NULL
		END
	ORDER BY 
		product_id, type_name, product_name
	`

	// ส่งค่า startOfMonth และ endOfMonth ที่คำนวณได้
	if err := db.Db.Raw(query, startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")).Scan(&report).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "ไม่สามารถดึงข้อมูลรายงานรายเดือนได้",
		})
	}

	return c.JSON(fiber.Map{"report": report})
}

func GetDailySaleProductReport(c *fiber.Ctx) error {
	startDate, endDate, categoryID, productID := c.Query("start_date"), c.Query("end_date"), c.Query("category_id"), c.Query("product_id")

	// ตรวจสอบวันที่
	if err := validateDateRange(startDate, endDate); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// แปลง endDate ให้เป็นสิ้นสุดวัน (23:59:59)
	endDate = endDate + " 23:59:59"
	startDate = startDate + " 00:00:00"
	log.Printf("Start of month: %s", startDate)
	log.Printf("End of month: %s", endDate)

	var filters []string
	var args []interface{}

	// default filter
	filters = append(filters, "s.status = 'paid'")
	filters = append(filters, "s.deleted_at IS NULL")
	filters = append(filters, "v.start_time BETWEEN ? AND ?")
	args = append(args, startDate, endDate)

	// 🔍 ถ้ามี category_id
	if categoryID != "" && categoryID != "all" {
		if categoryID == "game" {
			// ค่าเกมส์ (product_id = 1)
			filters = append(filters, "s.product_id = 1")
		} else {
			filters = append(filters, "p.category_id = ?")
			args = append(args, categoryID)
		}
	}

	// 🔍 ถ้ามี product_id
	if productID != "" && productID != "all" {
		filters = append(filters, "s.product_id = ?")
		args = append(args, productID)
	}

	// var report []struct {
	// 	TypeName     string  `json:"type_name"`      // ประเภท เช่น ค่าเกมส์, อาหาร
	// 	ProductName  string  `json:"product_name"`   // ชื่อสินค้า หรือชื่อโต๊ะ (กรณี product_id = 1)
	// 	PricePerUnit float64 `json:"price_per_unit"` // ราคาต่อหน่วย
	// 	Qty          float64 `json:"qty"`            // จำนวน
	// 	NetPrice     float64 `json:"net_price"`      // ราคาสุทธิ
	// }

	// query := `
	// SELECT
	// 	CASE
	// 		WHEN s.product_id = 1 THEN 'ค่าเกมส์'
	// 		ELSE c.name
	// 	END AS type_name,
	// 	CASE
	// 		WHEN s.product_id = 1 THEN
	// 			st.name || ' (' ||
	// 			CASE
	// 				WHEN v.table_type = 0 THEN 'ปกติ'
	// 				ELSE 'ซ้อม'
	// 			END || ')'
	// 		ELSE p.name
	// 	END AS product_name,
	// 	p.price as price_per_unit,
	// 	SUM(s.sell_quantity) AS qty,
	// 	SUM(s.net_price) AS net_price
	// FROM
	// 	services s
	// LEFT JOIN
	// 	visitations v ON s.visitation_id = v.id
	// LEFT JOIN
	// 	setting_tables st ON v.table_id = st.id
	// LEFT JOIN
	// 	products p ON s.product_id = p.id
	// LEFT JOIN
	// 	categories c ON p.category_id = c.id
	// WHERE
	// 	s.status = 'paid'
	// 	AND s.deleted_at IS NULL
	// 	AND v.start_time BETWEEN ? AND ?
	// GROUP BY
	// 	CASE
	// 		WHEN s.product_id = 1 THEN v.table_id || '_' || v.table_type
	// 		ELSE s.product_id
	// 	END,
	// 	CASE
	// 		WHEN s.product_id = 1 THEN v.table_type
	// 		ELSE NULL
	// 	END
	// ORDER BY
	// 	product_id, type_name, product_name
	// `

	// 🔨 ประกอบ SQL
	query := fmt.Sprintf(`
	SELECT 
		CASE 
			WHEN s.product_id = 1 THEN 'ค่าเกมส์'
			ELSE c.name
		END AS type_name,
		CASE 
			WHEN s.product_id = 1 THEN 
				st.name || ' (' || 
				CASE 
					WHEN v.table_type = 0 THEN 'ปกติ'
					ELSE 'ซ้อม'
				END || ')'
			ELSE p.name
		END AS product_name,
		p.price as price_per_unit,
		SUM(s.sell_quantity) AS qty,
		SUM(s.net_price) AS net_price
	FROM 
		services s
	LEFT JOIN visitations v ON s.visitation_id = v.id
	LEFT JOIN setting_tables st ON v.table_id = st.id
	LEFT JOIN products p ON s.product_id = p.id
	LEFT JOIN categories c ON p.category_id = c.id
	WHERE %s
	GROUP BY 
		CASE 
			WHEN s.product_id = 1 THEN v.table_id || '_' || v.table_type
			ELSE s.product_id
		END,
		CASE 
			WHEN s.product_id = 1 THEN v.table_type
			ELSE NULL
		END
	ORDER BY product_id, type_name, product_name
	`, strings.Join(filters, " AND "))

	var report []struct {
		TypeName     string  `json:"type_name"`
		ProductName  string  `json:"product_name"`
		PricePerUnit float64 `json:"price_per_unit"`
		Qty          float64 `json:"qty"`
		NetPrice     float64 `json:"net_price"`
	}

	if err := db.Db.Raw(query, args...).Scan(&report).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "ไม่สามารถดึงข้อมูลรายงานรายวันได้",
		})
	}
	// // ส่งค่า startOfMonth และ endOfMonth ที่คำนวณได้
	// if err := db.Db.Raw(query, startDate, endDate).Scan(&report).Error; err != nil {
	// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
	// 		"error": "ไม่สามารถดึงข้อมูลรายงานรายเดือนได้",
	// 	})
	// }

	return c.JSON(fiber.Map{"report": report})
}
