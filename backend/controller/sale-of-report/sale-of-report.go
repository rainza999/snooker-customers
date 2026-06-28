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
	TableName      string  `json:"table_name"`
	BillNumber     string  `json:"bill_number"`
	StartDate      string  `json:"start_date"`
	StartTime      string  `json:"start_time"`
	EndDate        string  `json:"end_date"`
	EndTime        string  `json:"end_time"`
	TotalBill      float64 `json:"total_bill"`
	CashAmount     float64 `json:"cash_amount"`
	TransferAmount float64 `json:"transfer_amount"`
	CreditAmount   float64 `json:"credit_amount"`
	Uuid           string  `json:"uuid"`
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
		BillCode       string    `json:"bill_code"`
		StartTime      time.Time `json:"start_time"`
		EndTime        time.Time `json:"end_time"`
		NetPrice       float64   `json:"net_price"`
		TotalCost      float64   `json:"total_cost"`
		TableID        uint      `json:"table_id"`
		PaidAmount     float64   `json:"paid_amount"`
		ChangeAmount   float64   `json:"change_amount"`
		CashAmount     float64   `json:"cash_amount"`
		TransferAmount float64   `json:"transfer_amount"`
		CreditAmount   float64   `json:"credit_amount"`
		Uuid           string    `json:"uuid"`
	}
	err := db.Db.Raw(`
		SELECT
			v.bill_code,
			v.start_time,
			v.end_time,
			v.net_price,
			v.total_cost,
			v.paid_amount,
			v.change_amount,
			v.table_id,
			v.uuid,
			COALESCE(SUM(CASE WHEN bp.method = 'cash' THEN bp.amount ELSE 0 END), 0) AS cash_amount,
			COALESCE(SUM(CASE WHEN bp.method = 'transfer' THEN bp.amount ELSE 0 END), 0) AS transfer_amount,
			COALESCE(SUM(CASE WHEN bp.method = 'credit' THEN bp.amount ELSE 0 END), 0) AS credit_amount
		FROM visitations v
		LEFT JOIN bill_payments bp
			ON bp.visitation_id = v.id
			AND bp.deleted_at IS NULL
			AND bp.is_active = 1
		WHERE (v.start_time BETWEEN ? AND ? )
			AND (v.is_paid = 1)
			AND v.deleted_at IS NULL
			AND v.is_active = 1
		GROUP BY v.id
		ORDER BY v.start_time ASC`, startDate, endDate).Scan(&visitations).Error
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// แปลงข้อมูล start_time ให้เป็นแยก "ปี-เดือน-วัน" และ "เวลา HH:MM"
	var reportData []VisitationReport
	for _, v := range visitations {
		reportData = append(reportData, VisitationReport{
			TableName:      getTableName(v.TableID), // ฟังก์ชันสำหรับดึงชื่อโต๊ะ
			BillNumber:     v.BillCode,
			StartDate:      v.StartTime.Format("2006-01-02"), // แปลงเป็น "ปี-เดือน-วัน"
			StartTime:      v.StartTime.Format("15:04"),      // แปลงเป็น "HH:MM"
			EndDate:        v.EndTime.Format("2006-01-02"),   // แปลงเป็น "ปี-เดือน-วัน"
			EndTime:        v.EndTime.Format("15:04"),        // แปลงเป็น "HH:MM"
			TotalBill:      v.NetPrice,
			CashAmount:     v.CashAmount,
			TransferAmount: v.TransferAmount,
			CreditAmount:   v.CreditAmount,
			Uuid:           v.Uuid,
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
		ID             uint    `json:"id"` // visitation_id เพื่อเอาไปใช้ค้นใน services
		BillCode       string  `json:"bill_code"`
		TableID        uint    `json:"table_id"`
		TableName      string  `json:"table_name"`
		StartTime      string  `json:"start_time"`
		EndTime        string  `json:"end_time"`
		NetPrice       float64 `json:"net_price"`
		PausedDuration float64 `json:"paused_duration"`
		PauseTime      string  `json:"pause_time"`
		TotalCost      float64 `json:"total_cost"`
		PaidAmount     float64 `json:"paid_amount"`
		ChangeAmount   float64 `json:"change_amount"`
		TableType      uint    `json:"table_type"`
		Price          float64 `json:"price"`
		Price2         float64 `json:"price2"`
		UseTime        int64   `json:"use_time"`
	}
	// Query ข้อมูลจาก table visitations โดยใช้ uuid
	err := db.Db.Raw(`SELECT visitations.id, bill_code, 
    table_id, start_time, end_time, net_price, total_cost, paid_amount, change_amount, table_type, setting_tables.name as table_name,
    setting_tables.price as price, 
    setting_tables.price2 as price2,
	pause_time,
	paused_duration,
	use_time

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
	// err = db.Db.Raw(`
	// 	SELECT
	// 		services.product_id,
	// 		products.name AS product_name,
	// 		services.sell_quantity,
	// 		services.total_cost,
	// 		services.net_price
	// 	FROM services
	// 	LEFT JOIN products ON services.product_id = products.id
	// 	WHERE services.visitation_id = ? and services.deleted_at is null and services.status = 'paid'`, visitation.ID).Scan(&serviceDetails).Error

	err = db.Db.Raw(`
	SELECT 
		services.product_id,
		products.name AS product_name,
		CASE 
			WHEN services.product_id = 1 THEN services.use_time
			ELSE services.sell_quantity
		END AS sell_quantity,
		services.total_cost,
		services.net_price
	FROM services
	LEFT JOIN products ON services.product_id = products.id
	WHERE services.visitation_id = ? 
	  AND services.deleted_at IS NULL 
	  AND services.status = 'paid'`,
		visitation.ID,
	).Scan(&serviceDetails).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "ไม่พบข้อมูลใน services",
		})
	}

	// ส่งข้อมูลกลับไปในรูปแบบ JSON
	paymentDetails, err := getBillPaymentDetails(visitation.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to load payment details",
		})
	}
	priceSegments, err := getBillPriceSegmentDetails(visitation.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to load price details",
		})
	}

	return c.JSON(fiber.Map{
		"visitation":     visitation,
		"serviceDetails": serviceDetails,
		"payments":       paymentDetails,
		"price_segments": priceSegments,
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
		Date       string  `json:"date"`
		GameFee    float64 `json:"game_fee"`
		FoodFee    float64 `json:"food_fee"`
		DrinkFee   float64 `json:"drink_fee"`
		Cat4Fee    float64 `json:"cat4_fee"`
		Cat2Fee    float64 `json:"cat2_fee"`
		Cat5678Fee float64 `json:"cat5678_fee"`
		TotalFee   float64 `json:"total_fee"`
	}

	query := `
    SELECT
  DATE(datetime(v.start_time, '+7 hours')) AS date,
  COALESCE(SUM(CASE WHEN s.product_id = 1 THEN s.net_price END), 0)                      AS game_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 3 THEN s.net_price END), 0)                     AS food_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 1 THEN s.net_price END), 0)                     AS drink_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 4 THEN s.net_price END), 0)                     AS cat4_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 2 THEN s.net_price END), 0)                     AS cat2_fee,
  COALESCE(SUM(CASE WHEN p.category_id IN (5,6,7,8) THEN s.net_price END), 0)            AS cat5678_fee,
  COALESCE(SUM(s.net_price), 0)                                                          AS total_fee
FROM visitations v
JOIN services s ON v.id = s.visitation_id
JOIN products p ON p.id = s.product_id
WHERE s.status = 'paid'
  AND s.deleted_at IS NULL
  AND v.start_time >= ?
  AND v.start_time <  ?
  AND v.deleted_at is null
GROUP BY DATE(datetime(v.start_time, '+7 hours'))
ORDER BY DATE(datetime(v.start_time, '+7 hours'));

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
				CASE 
			WHEN s.product_id = 1 THEN SUM(v.use_time)       -- 🕒 รวมเวลาทั้งหมดหน่วยวินาที
			ELSE SUM(s.sell_quantity)
		END AS qty,
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
    p.price AS price_per_unit,
CASE 
    WHEN s.product_id = 1 THEN 
        ROUND(SUM(v.use_time), 2)
    ELSE 
        SUM(s.sell_quantity)
END AS qty,

    SUM(s.net_price) AS net_price

FROM services s
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
ORDER BY s.product_id, type_name, product_name
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
	return c.JSON(fiber.Map{"report": report})
}

// ฟังก์ชันดึงข้อมูลรายงานยอดขายรายวัน
func GetDailySalesCloseReport(c *fiber.Ctx) error {
	startDate, endDate := c.Query("start_date"), c.Query("end_date")

	// ตรวจสอบวันที่
	if err := validateDateRange(startDate, endDate); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// แปลง endDate ให้เป็นสิ้นสุดวัน (23:59:59)
	endDate = endDate + " 23:59:59"

	// Query ข้อมูลจากฐานข้อมูล
	var visitations []struct {
		BillCode       string    `json:"bill_code"`
		StartTime      time.Time `json:"start_time"`
		EndTime        time.Time `json:"end_time"`
		NetPrice       float64   `json:"net_price"`
		TotalCost      float64   `json:"total_cost"`
		TableID        uint      `json:"table_id"`
		PaidAmount     float64   `json:"paid_amount"`
		ChangeAmount   float64   `json:"change_amount"`
		CashAmount     float64   `json:"cash_amount"`
		TransferAmount float64   `json:"transfer_amount"`
		CreditAmount   float64   `json:"credit_amount"`
		Uuid           string    `json:"uuid"`
	}
	// err := db.Db.Raw(`SELECT bill_code, start_time, end_time, net_price, total_cost, paid_amount, change_amount, table_id, uuid
	// 	FROM visitations WHERE (start_time BETWEEN ? AND ? )  and deleted_at is null and is_active = 0`, startDate, endDate).Scan(&visitations).Error
	err := db.Db.Raw(`
	SELECT 
		v.bill_code, 
		v.start_time, 
		v.end_time, 
		COALESCE(SUM(s.net_price), 0)  AS net_price, 
		COALESCE(SUM(s.total_cost), 0) AS total_cost,
		v.paid_amount, 
		v.change_amount, 
		v.table_id, 
		v.uuid,
		COALESCE(bp.cash_amount, 0) AS cash_amount,
		COALESCE(bp.transfer_amount, 0) AS transfer_amount,
		COALESCE(bp.credit_amount, 0) AS credit_amount
	FROM visitations v
	LEFT JOIN services s 
		ON v.id = s.visitation_id 
		AND s.deleted_at IS NULL 
		AND (s.status = 'paid' OR s.status = 'draft')
	LEFT JOIN (
		SELECT
			visitation_id,
			SUM(CASE WHEN method = 'cash' THEN amount ELSE 0 END) AS cash_amount,
			SUM(CASE WHEN method = 'transfer' THEN amount ELSE 0 END) AS transfer_amount,
			SUM(CASE WHEN method = 'credit' THEN amount ELSE 0 END) AS credit_amount
		FROM bill_payments
		WHERE deleted_at IS NULL
			AND is_active = 1
		GROUP BY visitation_id
	) bp ON bp.visitation_id = v.id
	WHERE 
		v.start_time BETWEEN ? AND ?
		AND v.deleted_at IS NULL
		AND v.is_active = 0
	GROUP BY 
		v.id, v.bill_code, v.start_time, v.end_time, v.paid_amount, 
		v.change_amount, v.table_id, v.uuid
	ORDER BY 
		v.start_time ASC
`, startDate, endDate).Scan(&visitations).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// แปลงข้อมูล start_time ให้เป็นแยก "ปี-เดือน-วัน" และ "เวลา HH:MM"
	var reportData []VisitationReport
	for _, v := range visitations {
		reportData = append(reportData, VisitationReport{
			TableName:      getTableName(v.TableID), // ฟังก์ชันสำหรับดึงชื่อโต๊ะ
			BillNumber:     v.BillCode,
			StartDate:      v.StartTime.Format("2006-01-02"), // แปลงเป็น "ปี-เดือน-วัน"
			StartTime:      v.StartTime.Format("15:04"),      // แปลงเป็น "HH:MM"
			EndDate:        v.EndTime.Format("2006-01-02"),   // แปลงเป็น "ปี-เดือน-วัน"
			EndTime:        v.EndTime.Format("15:04"),        // แปลงเป็น "HH:MM"
			TotalBill:      v.NetPrice,
			CashAmount:     v.CashAmount,
			TransferAmount: v.TransferAmount,
			CreditAmount:   v.CreditAmount,
			Uuid:           v.Uuid,
		})
	}

	// ตรวจสอบว่ามีข้อมูลหรือไม่ ถ้าไม่มีให้ส่ง response ที่เหมาะสมกลับไป
	if len(reportData) == 0 {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"visitations": nil})
	}

	return c.JSON(fiber.Map{"visitations": reportData})
}

// ฟังก์ชันสำหรับดึงรายละเอียดรายงานการขาย (detail)
func GetDailySalesCloseReportDetail(c *fiber.Ctx) error {
	// ดึง uuid จาก URL parameter
	uuid := c.Params("uuid")

	// Query ข้อมูลของบิลจาก table visitations
	var visitation struct {
		ID             uint    `json:"id"` // visitation_id เพื่อเอาไปใช้ค้นใน services
		BillCode       string  `json:"bill_code"`
		TableID        uint    `json:"table_id"`
		TableName      string  `json:"table_name"`
		StartTime      string  `json:"start_time"`
		EndTime        string  `json:"end_time"`
		NetPrice       float64 `json:"net_price"`
		PausedDuration float64 `json:"paused_duration"`
		PauseTime      string  `json:"pause_time"`
		TotalCost      float64 `json:"total_cost"`
		PaidAmount     float64 `json:"paid_amount"`
		ChangeAmount   float64 `json:"change_amount"`
		TableType      uint    `json:"table_type"`
		Price          float64 `json:"price"`
		Price2         float64 `json:"price2"`
		UseTime        int64   `json:"use_time"`
	}
	// Query ข้อมูลจาก table visitations โดยใช้ uuid
	err := db.Db.Raw(`SELECT visitations.id, bill_code, 
    table_id, start_time, end_time, net_price, total_cost, paid_amount, change_amount, table_type, setting_tables.name as table_name,
    setting_tables.price as price, 
    setting_tables.price2 as price2,
	pause_time,
	paused_duration,
	use_time


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
	WHERE 
		services.visitation_id = ? 
		AND services.deleted_at IS NULL 
		AND (services.status = 'paid' OR services.status = 'draft')
`, visitation.ID).Scan(&serviceDetails).Error

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "ไม่พบข้อมูลใน services",
		})
	}

	// ส่งข้อมูลกลับไปในรูปแบบ JSON
	paymentDetails, err := getBillPaymentDetails(visitation.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to load payment details",
		})
	}
	priceSegments, err := getBillPriceSegmentDetails(visitation.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to load price details",
		})
	}

	return c.JSON(fiber.Map{
		"visitation":     visitation,
		"serviceDetails": serviceDetails,
		"payments":       paymentDetails,
		"price_segments": priceSegments,
	})
}

func GetMonthlySalesCloseReport(c *fiber.Ctx) error {
	// ดึงเดือนที่เลือกจาก query parameter
	selectedMonth := c.Query("month") // รูปแบบ "YYYY-MM"

	// Query ดึงข้อมูลยอดขายตามเดือน
	var report []struct {
		Date       string  `json:"date"`
		GameFee    float64 `json:"game_fee"`
		FoodFee    float64 `json:"food_fee"`
		DrinkFee   float64 `json:"drink_fee"`
		Cat4Fee    float64 `json:"cat4_fee"`
		Cat2Fee    float64 `json:"cat2_fee"`
		Cat5678Fee float64 `json:"cat5678_fee"`
		TotalFee   float64 `json:"total_fee"`
	}

	query := `
SELECT
  DATE(datetime(v.start_time, '+7 hours')) AS date,
  COALESCE(SUM(CASE WHEN s.product_id = 1 THEN s.net_price END), 0)                      AS game_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 3 THEN s.net_price END), 0)                     AS food_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 1 THEN s.net_price END), 0)                     AS drink_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 4 THEN s.net_price END), 0)                     AS cat4_fee,
  COALESCE(SUM(CASE WHEN p.category_id = 2 THEN s.net_price END), 0)                     AS cat2_fee,
  COALESCE(SUM(CASE WHEN p.category_id IN (5,6,7,8) THEN s.net_price END), 0)            AS cat5678_fee,
  COALESCE(SUM(s.net_price), 0)                                                          AS total_fee
FROM visitations v
JOIN services s ON v.id = s.visitation_id
JOIN products p ON p.id = s.product_id
WHERE 
  s.deleted_at IS NULL
  and v.deleted_at is null
  AND s.status = 'draft'
  AND v.start_time >= ?
  AND v.start_time < ?
GROUP BY DATE(datetime(v.start_time, '+7 hours'))
ORDER BY DATE(datetime(v.start_time, '+7 hours'));
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

func GetDailyPaymentReport(c *fiber.Ctx) error {
	startDate, endDate := c.Query("start_date"), c.Query("end_date")
	if err := validateDateRange(startDate, endDate); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return paymentReportByRange(c, startDate+" 00:00:00", endDate+" 23:59:59")
}

func GetMonthlyPaymentReport(c *fiber.Ctx) error {
	selectedMonth := c.Query("month")
	startOfMonth, err := time.Parse("2006-01", selectedMonth)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid month format"})
	}
	endOfMonth := startOfMonth.AddDate(0, 1, 0)
	return paymentReportByRange(c, startOfMonth.Format("2006-01-02 15:04:05"), endOfMonth.Format("2006-01-02 15:04:05"))
}

func paymentReportByRange(c *fiber.Ctx, startAt string, endAt string) error {
	var rows []struct {
		BillCode  string    `json:"bill_code"`
		TableName string    `json:"table_name"`
		PaidAt    time.Time `json:"paid_at"`
		Method    string    `json:"method"`
		Amount    float64   `json:"amount"`
		Note      string    `json:"note"`
	}
	err := db.Db.Raw(`
		SELECT
			v.bill_code,
			st.name AS table_name,
			bp.paid_at,
			bp.method,
			bp.amount,
			bp.note
		FROM bill_payments bp
		JOIN visitations v ON v.id = bp.visitation_id
		LEFT JOIN setting_tables st ON st.id = v.table_id
		WHERE bp.deleted_at IS NULL
			AND bp.is_active = 1
			AND v.deleted_at IS NULL
			AND v.is_paid = 1
			AND bp.paid_at >= ?
			AND bp.paid_at <= ?
		ORDER BY bp.paid_at ASC, bp.id ASC
	`, startAt, endAt).Scan(&rows).Error
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load payment report"})
	}

	details := make([]fiber.Map, 0, len(rows))
	totals := map[string]float64{
		"cash":     0,
		"transfer": 0,
		"credit":   0,
	}
	total := 0.0
	for _, row := range rows {
		totals[row.Method] = totals[row.Method] + row.Amount
		total += row.Amount
		details = append(details, fiber.Map{
			"bill_code":  row.BillCode,
			"table_name": row.TableName,
			"paid_at":    row.PaidAt.Format("2006-01-02 15:04:05"),
			"method":     row.Method,
			"amount":     row.Amount,
			"note":       row.Note,
		})
	}
	return c.JSON(fiber.Map{
		"details": details,
		"totals": fiber.Map{
			"cash":     totals["cash"],
			"transfer": totals["transfer"],
			"credit":   totals["credit"],
			"all":      total,
		},
	})
}

type BillPaymentReport struct {
	Method string  `json:"method"`
	Amount float64 `json:"amount"`
	Note   string  `json:"note"`
	PaidAt string  `json:"paid_at"`
}

type BillPriceSegmentReport struct {
	Source          string  `json:"source"`
	PromotionName   string  `json:"promotion_name"`
	SegmentStart    string  `json:"segment_start"`
	SegmentEnd      string  `json:"segment_end"`
	DurationSeconds int64   `json:"duration_seconds"`
	TableType       uint8   `json:"table_type"`
	UnitPrice       float64 `json:"unit_price"`
	Amount          float64 `json:"amount"`
}

func getBillPaymentDetails(visitationID uint) ([]BillPaymentReport, error) {
	var rows []struct {
		Method string
		Amount float64
		Note   string
		PaidAt time.Time
	}
	err := db.Db.Raw(`
		SELECT method, amount, note, paid_at
		FROM bill_payments
		WHERE visitation_id = ?
			AND deleted_at IS NULL
			AND is_active = 1
		ORDER BY id ASC
	`, visitationID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	reports := make([]BillPaymentReport, 0, len(rows))
	for _, row := range rows {
		reports = append(reports, BillPaymentReport{
			Method: row.Method,
			Amount: row.Amount,
			Note:   row.Note,
			PaidAt: row.PaidAt.Format("2006-01-02 15:04:05"),
		})
	}
	return reports, nil
}

func getBillPriceSegmentDetails(visitationID uint) ([]BillPriceSegmentReport, error) {
	var rows []struct {
		Source          string
		PromotionName   string
		SegmentStart    time.Time
		SegmentEnd      time.Time
		DurationSeconds int64
		TableType       uint8
		UnitPrice       float64
		Amount          float64
	}
	err := db.Db.Raw(`
		SELECT
			bps.source,
			COALESCE(p.name, '') AS promotion_name,
			bps.segment_start,
			bps.segment_end,
			bps.duration_seconds,
			bps.table_type,
			bps.unit_price,
			bps.amount
		FROM bill_price_segments bps
		LEFT JOIN promotions p ON p.id = bps.promotion_id
		WHERE bps.visitation_id = ?
			AND bps.deleted_at IS NULL
		ORDER BY bps.segment_start ASC, bps.id ASC
	`, visitationID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	reports := make([]BillPriceSegmentReport, 0, len(rows))
	for _, row := range rows {
		reports = append(reports, BillPriceSegmentReport{
			Source:          row.Source,
			PromotionName:   row.PromotionName,
			SegmentStart:    row.SegmentStart.Format("2006-01-02 15:04:05"),
			SegmentEnd:      row.SegmentEnd.Format("2006-01-02 15:04:05"),
			DurationSeconds: row.DurationSeconds,
			TableType:       row.TableType,
			UnitPrice:       row.UnitPrice,
			Amount:          row.Amount,
		})
	}
	return reports, nil
}
