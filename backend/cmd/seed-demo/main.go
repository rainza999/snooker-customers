package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rainza999/fiber-test/seed/demo"
)

func main() {
	var config demo.Config
	flag.StringVar(&config.OutputPath, "output", "../tmp/demo-seed/snooker.db", "output SQLite database path")
	flag.StringVar(&config.AdminUsername, "admin-username", "admin", "initial admin username")
	flag.StringVar(&config.AdminPassword, "admin-password", os.Getenv("DEMO_ADMIN_PASSWORD"), "initial admin password; prefer DEMO_ADMIN_PASSWORD")
	flag.StringVar(&config.CloseTablePassword, "close-table-password", os.Getenv("DEMO_CLOSE_TABLE_PASSWORD"), "close-table password; defaults to admin password")
	flag.StringVar(&config.EditReportPassword, "edit-report-password", os.Getenv("DEMO_EDIT_REPORT_PASSWORD"), "edit-report password; defaults to close-table password")
	flag.StringVar(&config.CompanyName, "company-name", "YS Snooker Demo", "company name")
	flag.StringVar(&config.DivisionName, "division-name", "สาขาทดลอง", "division name")
	flag.BoolVar(&config.Force, "force", false, "replace an existing output database")
	flag.Parse()

	summary, err := demo.Generate(config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Clean demo database created")
	fmt.Println("output:", summary.OutputPath)
	fmt.Println("admin username:", config.AdminUsername)
	fmt.Println("tables:", summary.TableCount)
	fmt.Println("categories:", summary.CategoryCount)
	fmt.Println("products:", summary.ProductCount)
	fmt.Println("suppliers:", summary.SupplierCount)
	fmt.Println("opening stock items:", summary.OpeningStockItemCount)
}
