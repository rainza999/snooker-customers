package db

// import (
// 	"gorm.io/driver/sqlite"
// 	"gorm.io/gorm"
// 	"gorm.io/gorm/logger"
// )

// var Db *gorm.DB
// var err error

// func InitDB() {
// 	// dsn := os.Getenv("SQLITE_DSN")
// 	dsn := "file:snooker.db?_pragma_key=rainza999"
// 	// Db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
// 	Db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
// 		Logger: logger.Default.LogMode(logger.Info), // เปิด Debug Logging
// 	})
// 	if err != nil {
// 		panic("fail to connect database")
// 	}

// }

import (
	"log"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var Db *gorm.DB

func InitDB() {
	execPath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	execDir := filepath.Dir(execPath)
	dbPath := filepath.Join(execDir, "snooker.db")

	// ใส่ key เข้าไปใน DSN เหมือนเดิม
	dsn := "file:" + dbPath + "?_pragma_key=rainza999"

	log.Println("Opening DB at:", dbPath)

	Db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info), // เปิด Debug Logging
	})
	if err != nil {
		panic("fail to connect database: " + err.Error())
	}
}

// // Find คือฟังก์ชันที่ใช้ Find จาก Db
// func Find(dest interface{}) *gorm.DB {
// 	return Db.Find(dest)
// }

// // SelectFromTable คือฟังก์ชันที่ใช้สร้าง query ที่มี Select, Where, First
// func SelectFromTable(table string, columns []string, condition string, args ...interface{}) *gorm.DB {
// 	return Db.Table(table).Select(columns).Where(condition, args...)
// }

// // FindFromTable คือฟังก์ชันที่ใช้ Find จาก query ที่สร้างขึ้น
// func FindFromTable(query *gorm.DB, dest interface{}) *gorm.DB {
// 	return query.First(dest)
// }
