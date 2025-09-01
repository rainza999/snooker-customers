package helper

import (
	"log"

	// "github.com/dgrijalva/jwt-go"

	"context"

	"github.com/rainza999/fiber-test/db"
	model "github.com/rainza999/fiber-test/models"
)

// RoleMenuCache represents the structure of cached menu data for a specific role
type RoleMenuCache struct {
	RoleID   int         `json:"role_id"`
	RoleName string      `json:"role_name"`
	Data     interface{} `json:"data"`
}

func GetMenusByRole(roleID uint) ([]Menu, error) {
	// var menus []Menu
	// // คิวรีเมนูจากฐานข้อมูลที่เกี่ยวข้องกับ roleID
	// err := db.Db.Where("role_id = ?", roleID).Find(&menus).Error
	// if err != nil {
	//     return nil, err
	// }
	// return menus, nil

	var menuIDs []uint

	// ดึง menu_id ที่ผู้ใช้มีสิทธิ์เข้าถึง
	db.Db.Table("role_has_permissions").
		Where("role_has_permissions.role_id", roleID).
		Joins("JOIN permissions ON role_has_permissions.permission_id = permissions.id").
		Distinct("permissions.menu_id").
		Pluck("permissions.menu_id", &menuIDs)

	// ดึงรายการเมนูทั้งหมด
	menusRecords := getMenu()

	// สร้าง slice สำหรับเก็บข้อมูลเมนูที่ผู้ใช้มีสิทธิ์เข้าถึง
	var menus []Menu
	for _, menusRecord := range menusRecords {
		// ตรวจสอบว่าผู้ใช้มีสิทธิ์ในเมนูนี้หรือไม่ (Route != "#" หมายถึงมีสิทธิ์)
		if menusRecord.Route == "#" || in_array(menuIDs, menusRecord.ID) {
			// ดึง SubMenu (ถ้ามี)
			menu := getSubMenuByRole(menusRecord, menuIDs)

			// ตรวจสอบว่าเมนูย่อยว่างหรือไม่ ถ้าเมนูย่อยว่าง และ HasSub == 1 จะไม่เพิ่มเมนูนี้เข้า slice
			if menu.HasSub == 1 && len(menu.Sub) == 0 {
				continue // ข้ามเมนูที่ไม่มีเมนูย่อย
			}

			// เพิ่มเมนูที่ผ่านเงื่อนไขเข้าไปใน slice
			menus = append(menus, menu)
		}
	}

	// ลบเมนูย่อยที่ไม่มีข้อมูล (ID == 0)
	for key := range menus {
		// ลบเมนูย่อยที่มี ID == 0
		filteredSubMenu := []Menu{}
		for _, subMenu := range menus[key].Sub {
			if subMenu.ID != 0 {
				filteredSubMenu = append(filteredSubMenu, subMenu)
			}
		}
		menus[key].Sub = filteredSubMenu

		// ถ้าหลังจากลบเมนูย่อยแล้วเมนูไม่มีเมนูย่อยและมี HasSub == 1 ก็จะตั้งค่า SubMenu เป็น nil
		if len(menus[key].Sub) == 0 && menus[key].HasSub == 1 {
			menus[key].Sub = nil // ตั้งค่า SubMenu เป็น nil
		}
	}

	log.Println(menus)
	return menus, nil
}

func GetMenuByRole(roleID int) interface{} {
	var menuIDs []uint

	// ดึง menu_id ที่ผู้ใช้มีสิทธิ์เข้าถึง
	db.Db.Table("role_has_permissions").
		Where("role_has_permissions.role_id", roleID).
		Joins("JOIN permissions ON role_has_permissions.permission_id = permissions.id").
		Distinct("permissions.menu_id").
		Pluck("permissions.menu_id", &menuIDs)

	// ดึงรายการเมนูทั้งหมด
	menusRecords := getMenu()

	// สร้าง slice สำหรับเก็บข้อมูลเมนูที่ผู้ใช้มีสิทธิ์เข้าถึง
	var menus []Menu
	for _, menusRecord := range menusRecords {
		// ตรวจสอบว่าผู้ใช้มีสิทธิ์ในเมนูนี้หรือไม่ (Route != "#" หมายถึงมีสิทธิ์)
		if menusRecord.Route == "#" || in_array(menuIDs, menusRecord.ID) {
			// ดึง SubMenu (ถ้ามี)
			menu := getSubMenuByRole(menusRecord, menuIDs)

			// ตรวจสอบว่าเมนูย่อยว่างหรือไม่ ถ้าเมนูย่อยว่าง และ HasSub == 1 จะไม่เพิ่มเมนูนี้เข้า slice
			if menu.HasSub == 1 && len(menu.Sub) == 0 {
				continue // ข้ามเมนูที่ไม่มีเมนูย่อย
			}

			// เพิ่มเมนูที่ผ่านเงื่อนไขเข้าไปใน slice
			menus = append(menus, menu)
		}
	}

	// ลบเมนูย่อยที่ไม่มีข้อมูล (ID == 0)
	for key := range menus {
		// ลบเมนูย่อยที่มี ID == 0
		filteredSubMenu := []Menu{}
		for _, subMenu := range menus[key].Sub {
			if subMenu.ID != 0 {
				filteredSubMenu = append(filteredSubMenu, subMenu)
			}
		}
		menus[key].Sub = filteredSubMenu

		// ถ้าหลังจากลบเมนูย่อยแล้วเมนูไม่มีเมนูย่อยและมี HasSub == 1 ก็จะตั้งค่า SubMenu เป็น nil
		if len(menus[key].Sub) == 0 && menus[key].HasSub == 1 {
			menus[key].Sub = nil // ตั้งค่า SubMenu เป็น nil
		}
	}

	log.Println(menus)
	return menus
}

// func getMenuByRole(roleID int) interface{} {
// 	var menuIDs []uint

// 	db.Db.Table("role_has_permissions").
// 		Where("role_has_permissions.role_id", roleID).
// 		Joins("JOIN permissions ON role_has_permissions.permission_id = permissions.id").
// 		Distinct("permissions.menu_id").
// 		Pluck("permissions.menu_id", &menuIDs)

// 	// log.Println("menuIDs")
// 	// log.Println(roleID)
// 	// log.Println(menuIDs)

// 	menusRecords := getMenu()

// 	// log.Println("menusRecords")
// 	// log.Println(menusRecords)
// 	//**
// 	//[
// 	//{1 การคิดเงิน /point-of-sale 0 0 0 1 AttachMoneyIcon []}
// 	//{2 จัดการข้อมูล # 0 0 1 2  [{3 ข้อมูลโต๊ะสนุ๊ก /setting-table 1 2 0 1 AppsIcon []} {4 ข้อมูลเวลา /setting-timer 1 2 0 2 Alar           rmAddIcon []}]}
// 	//{5 จัดการผู้ใช้งาน # 0 0 1 3  [{6 รายชื่อผู้ใช้งาน /users 1 5 0 1 ManageAccountsIcon []} {7 สิทธิ์การใช้งาน /roles 1 5 0 2 SecurityIcon []}]}
// 	//]
// 	//
// 	log.Println("==============================")

// 	var menus []Menu
// 	for key, menusRecord := range menusRecords {
// 		log.Println("menusRecord On getMenuByRole")
// 		log.Println(menusRecord.Name)
// 		log.Println(menusRecord.HasSub)
// 		log.Println(menusRecord.Relation)
// 		log.Println(menusRecord.Route)
// 		log.Println(menusRecord.ID)

// 		if menusRecord.Route == "#" || in_array(menuIDs, menusRecord.ID) {
// 			// log.Println("ในนี้คือเงื่อนไข if")
// 			// log.Println("ปิดไปก่อน")
// 			// log.Println("menusRecord")
// 			// log.Println(menusRecord)
// 			// log.Println("permissionMenus")
// 			// log.Println(menuIDs)
// 			// log.Println("ปิดทั้งหมด")
// 			// subMenu := getSubMenuByRole(menusRecord, menuIDs)

// 			menus = append(menus, getSubMenuByRole(menusRecord, menuIDs))

// 			log.Println("NNNNNNNNNNNNNNNNNNNNNNNNNN")
// 			log.Println(key)
// 			log.Println(menus)

// 			if key >= 0 && key < len(menus) {
// 				if len(menus[key].Sub) == 0 && menus[key].HasSub == 1 {
// 					// ใช้ append ร่วมกับ syntax ... เพื่อลบ element ที่ไม่ต้องการ
// 					menus[key].Sub = append(menus[key].Sub[:key], menus[key].Sub[key+1:]...)
// 					key-- // ลดค่า key เพื่อที่จะไม่ไปทำ iteration ที่ถูกลบไปแล้ว
// 				} else {
// 					// สร้าง slice ใหม่ที่มีค่าเป็น nil
// 					for _, subMenu := range menus[key].Sub {
// 						if subMenu.ID == 0 {
// 							menus[key].Sub = nil
// 						}
// 					}
// 				}
// 			}
// 			// if len(menus[key].Sub) == 0 && menus[key].HasSub == 1 {
// 			// 	// ใช้ append ร่วมกับ syntax ... เพื่อลบ element ที่ไม่ต้องการ
// 			// 	log.Println("if อยู่ในเงื่อนไข")
// 			// 	menus[key].Sub = append(menus[key].Sub[:key], menus[key].Sub[key+1:]...)
// 			// 	key-- // ลดค่า key เพื่อที่จะไม่ไปทำ iteration ที่ถูกลบไปแล้ว
// 			// } else {
// 			// 	// สร้าง slice ใหม่ที่มีค่าเป็น nil
// 			// 	log.Println("else อยู่ในเงื่อนไข")
// 			// 	log.Println(menus[key].Sub)
// 			// 	for _, subMenu := range menus[key].Sub {
// 			// 		log.Println("ID:", subMenu.ID)

// 			// 		if subMenu.ID == 0 {
// 			// 			menus[key].Sub = nil
// 			// 		} else {
// 			// 			// หรือถ้าคุณต้องการให้ Sub เป็น slice ว่าง
// 			// 			// menus[key].Sub = make([]Menu, 0)
// 			// 		}
// 			// 	}
// 			// 	log.Println("else อยู่ในเงื่อนไขปิด")
// 			// 	// menus[key].Sub = nil

// 			// }
// 		}
// 	}

// 	log.Println(menus)
// 	log.Println("menus")
// 	log.Println("================================")

// 	return menus
// }

func getSubMenuByRole(menusRecord Menu, menuIDs []uint) Menu {
	subMenus := menusRecord

	// log.Println("getSubMenuByRole")
	// log.Println(menusRecord)        //{2 จัดการข้อมูล # 0 0 1 2  [{3 ข้อมูลโต๊ะสนุ๊ก /setting-table 1 2 0 1 AppsIcon []} {4 ข้อมูลเวลา /setting-timer 1 2 0 2 AlarmAddIcon []}]}
	// log.Println(menusRecord.HasSub) //1
	// log.Println("=====================================")

	if menusRecord.HasSub == 1 {
		for key, menuSubRecord := range menusRecord.Sub {
			log.Println(in_array(menuIDs, menuSubRecord.ID))
			if menuSubRecord.Route == "#" || in_array(menuIDs, menuSubRecord.ID) {
				subMenus.Sub[key] = getSubMenuByRole(menuSubRecord, menuIDs)
				log.Println("test")
				log.Println(subMenus.Sub[key])
				log.Println(subMenus.Sub)
				log.Println("end test")
				if len(subMenus.Sub[key].Sub) == 0 && subMenus.Sub[key].HasSub == 1 {
					subMenus.Sub = append(subMenus.Sub[:key], subMenus.Sub[key+1:]...)
					key--
					log.Println("kuy111")
				}

				// subMenus.Sub = make([]Menu, 0)
				// subMenus.Sub[key].Sub = append(subMenus.Sub[key].Sub, getSubMenuByRole(menuSubRecord, menuIDs))

				// if len(subMenus.Sub[key].Sub) == 0 && subMenus.Sub[key].HasSub == 1 {
				// 	subMenus.Sub[key] = Menu{}
				// }
			} else {
				log.Println("kuy")
				subMenus.Sub[key] = Menu{}
			}

			if len(subMenus.Sub) == 0 {
				log.Println("len มัน = 0 นะ ข้อมูลจะหายมั้ย")
				subMenus.Sub = nil
			}
		}
		log.Println("subMenus นะ")
		log.Println(subMenus)
		return subMenus
	}
	log.Println("menusRecordxxxxxxxxxx")
	log.Println(menusRecord)
	return menusRecord
}

func in_array(slice []uint, value uint) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

type Menu struct {
	ID       uint
	Name     string
	Route    string
	Level    uint8
	Relation uint `gorm:"foreignKey:ID;references:ID;default:NULL"`
	HasSub   uint `gorm:"default:0"`
	Order    uint8
	Icon     string `gorm:"default:NULL"`
	Sub      []Menu
}

func getMenu() []Menu {
	var mainMenus []model.Menu
	db.Db.Select("id", "name", "route", "level", "relation", "order", "has_sub", "icon").
		Where("is_active = ?", true).
		Where("level = ?", 0).
		Order("\"menus\".\"order\"").
		Find(&mainMenus)

	var menus []Menu

	for _, mainMenu := range mainMenus {

		// ตรวจสอบเงื่อนไขเพิ่มเติมสำหรับ route = /setting-system และ id = 15
		if mainMenu.Route == "/setting-system" && mainMenu.ID == 15 {
			var firstTime bool
			db.Db.Table("setting_systems").Select("first_time").Limit(1).Scan(&firstTime)

			// ถ้า first_time เป็น false ให้ข้ามไปยังเมนูถัดไป
			if !firstTime {
				continue
			}
		}

		menu := Menu{
			ID:       mainMenu.ID,
			Name:     mainMenu.Name,
			Route:    mainMenu.Route,
			Level:    mainMenu.Level,
			Relation: mainMenu.Relation,
			HasSub:   mainMenu.HasSub,
			Order:    mainMenu.Order,
			Icon:     mainMenu.Icon,
		}

		if mainMenu.HasSub == 1 {
			level := mainMenu.Level + 1
			subMenu := getSubMenu(mainMenu.ID, level)
			menu.Sub = subMenu
		}

		menus = append(menus, menu)
	}
	return menus
}

func getSubMenu(menuID uint, level uint8) []Menu {
	var menuRecords []model.Menu
	db.Db.Select("id", "name", "route", "level", "relation", "order", "has_sub", "icon").
		Where("is_active = ?", true).
		Where("relation = ?", menuID).
		Where("level = ?", level).
		Order("\"menus\".\"order\"").
		Find(&menuRecords)

	var subMenux []Menu

	for _, menuRecord := range menuRecords {
		menu := Menu{
			ID:       menuRecord.ID,
			Name:     menuRecord.Name,
			Route:    menuRecord.Route,
			Level:    menuRecord.Level,
			Relation: menuRecord.Relation,
			HasSub:   menuRecord.HasSub,
			Order:    menuRecord.Order,
			Icon:     menuRecord.Icon,
		}

		if menuRecord.HasSub == 1 {
			level := menuRecord.Level + 1
			subMenu := getSubMenu(menuRecord.ID, level)
			menu.Sub = subMenu
		}

		subMenux = append(subMenux, menu)
	}

	return subMenux
}
func GetMenuData(roleID int) (*RoleMenuCache, error) {
	log.Println("GetMenuData for roleID:", roleID)

	// ดึงข้อมูลเมนูและข้อมูล role โดยตรง
	menuData, err := SetMenuData(roleID)
	if err != nil {
		log.Println("Error getting menu data:", err)
		return nil, err
	}

	return menuData, nil
}

// func GetMenuCache(ctx context.Context, roleID int) ([]byte, error) {
// 	log.Println("XXXXX____1")
// 	log.Println("XXXXX____2")
// 	log.Println("GetMenuCache for roleID:", roleID)
// 	// cacheKey := fmt.Sprintf("menus_%d", roleID)

// 	// // ดึงข้อมูลจาก Redis
// 	// cachedData, err := client.Get(ctx, cacheKey).Bytes()
// 	// if err == redis.Nil {
// 	// log.Println("Cache not found for key:", cacheKey)
// 	// client := redis.NewClient(&redis.Options{
// 	// 	Addr:     "redis:6379",
// 	// 	Password: "", // ใส่ password ถ้ามี
// 	// 	DB:       0,
// 	// })

// 	// // ctx := context.Background()
// 	if err := GenerateCacheMenu(ctx, client); err != nil {
// 		log.Fatal(err)
// 	}
// 	return client.Get(ctx, cacheKey).Bytes(), nil
// 	// return nil, nil // คืนค่า nil เมื่อไม่พบ cache แทนการคืน error
// 	// } else if err != nil {
// 	// 	log.Println("Error fetching cache:", err)
// 	// 	return nil, err
// 	// }

// 	// return cachedData, nil
// }

// func GetMenuCache(ctx context.Context, client *redis.Client, roleID int) error {
// 	// สร้าง dynamic cache key จาก roleID
// 	cacheKey := fmt.Sprintf("menus_%d", roleID)

// 	// ใช้คำสั่ง GET เพื่อดึงข้อมูลจาก Redis
// 	cachedData, err := client.Get(ctx, cacheKey).Result()
// 	if err == redis.Nil {
// 		// กรณีไม่พบข้อมูลใน Redis
// 		log.Println("Cache not found for key:", cacheKey)
// 	} else if err != nil {
// 		// เกิด error ในกรณีอื่น ๆ
// 		return err
// 	} else {
// 		// กรณีพบข้อมูลใน Redis
// 		log.Println("Cached data found:", cachedData)
// 	}

// 	// ตรงนี้คุณสามารถดำเนินการต่อไปตามที่คุณต้องการ เช่น แปลง cachedData เป็นโครงสร้างข้อมูลที่ต้องการ

// 	return nil
// }

// func getMenuCache(ctx context.Context, client *redis.Client, roleID int) error {

// }

// setCacheMenu สร้างและเก็บข้อมูล cache ใน Redis
// func SetCacheMenu(ctx context.Context, client *redis.Client, roleID int) error {
// 	log.Println("XXXXX____2")

// 	// สมมติว่าคุณมีฟังก์ชัน getMenuByRole ที่ดึงข้อมูลเมนูตาม roleID
// 	menuByRole := GetMenuByRole(roleID)

// 	// ดึงชื่อ role จากฐานข้อมูล
// 	var roleName string
// 	db.Db.Table("roles").
// 		Where("roles.id = ?", roleID).
// 		Pluck("roles.name", &roleName)

// 	// สร้างโครงสร้างข้อมูลสำหรับ cache
// 	cacheMenu := RoleMenuCache{
// 		RoleID:   roleID,
// 		RoleName: roleName,
// 		Data:     menuByRole,
// 	}

// 	// สร้าง cache key
// 	cacheKey := fmt.Sprintf("menus_%d", roleID)

// 	// แปลงโครงสร้างข้อมูลเป็น JSON เพื่อเก็บใน Redis
// 	cacheData, err := json.Marshal(cacheMenu)
// 	if err != nil {
// 		log.Println("Error marshalling cache data:", err)
// 		return err
// 	}

// 	// ตั้งค่า expiration ไม่มีวันหมดอายุ (-1) หรือสามารถตั้ง TTL ตามที่ต้องการ
// 	err = client.Set(ctx, cacheKey, cacheData, -1).Err()
// 	if err != nil {
// 		log.Println("Error setting cache:", err)
// 		return err
// 	}

// 	log.Println("Cache set for key:", cacheKey)
// 	return nil
// }

func SetNotCacheMenu(ctx context.Context, roleID int) (RoleMenuCache, error) {
	log.Println("XXXXX____3")

	// ดึงเมนูตาม role จากฐานข้อมูลโดยตรง
	menuByRole := GetMenuByRole(roleID)

	// ดึงชื่อ role จากฐานข้อมูล
	var roleName string
	if err := db.Db.Table("roles").
		Where("roles.id = ?", roleID).
		Pluck("roles.name", &roleName).Error; err != nil {
		log.Println("Error fetching role name from database:", err)
		return RoleMenuCache{}, err
	}

	// สร้างโครงสร้างข้อมูลเมนูตาม role
	cacheMenu := RoleMenuCache{
		RoleID:   roleID,
		RoleName: roleName,
		Data:     menuByRole,
	}

	return cacheMenu, nil
}
func SetMenuData(roleID int) (*RoleMenuCache, error) {
	// สมมติว่าคุณมีฟังก์ชัน getMenuByRole ที่ดึงข้อมูลเมนูตาม roleID
	menuByRole := GetMenuByRole(roleID)

	// ดึงชื่อ role จากฐานข้อมูล
	var roleName string
	err := db.Db.Table("roles").
		Where("roles.id = ?", roleID).
		Pluck("roles.name", &roleName).Error
	if err != nil {
		log.Println("Error fetching role name:", err)
		return nil, err
	}

	// สร้างโครงสร้างข้อมูลสำหรับการใช้งาน
	menuData := &RoleMenuCache{
		RoleID:   roleID,
		RoleName: roleName,
		Data:     menuByRole,
	}

	log.Println("Data set for role:", roleID)
	return menuData, nil
}

func GenerateMenuData() ([]*RoleMenuCache, error) {
	roles := []int{1, 2, 3, 4} // รหัส Role ทั้งหมด
	var menuDataList []*RoleMenuCache

	for _, roleID := range roles {
		menuData, err := SetMenuData(roleID)
		if err != nil {
			return nil, err
		}
		menuDataList = append(menuDataList, menuData)
	}

	return menuDataList, nil
}

// generateCacheMenu สร้างและ set cache สำหรับทุก Role
// func GenerateCacheMenu(ctx context.Context, client *redis.Client) error {
// 	roles := []int{1, 2, 3, 4} // รหัส Role ทั้งหมด

// 	for _, roleID := range roles {
// 		if err := SetCacheMenu(ctx, client, roleID); err != nil {
// 			return err
// 		}
// 	}

// 	return nil
// }
