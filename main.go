package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/johansundell/cocapi"
)

var db *sql.DB
var mysqlUser, mysqlPass, mysqlDb, mysqlHost string
var queryInsertUpdateMember = `INSERT INTO members (tag, name, created, last_updated, active) VALUES (?, ?, null, null, 1) ON DUPLICATE KEY UPDATE member_id=LAST_INSERT_ID(member_id), last_updated = NOW(), active = 1`

func init() {

	mysqlDb = "cocsniffer"
	mysqlHost = "localhost"
	mysqlUser = os.Getenv("MYSQL_USER")
	mysqlPass = os.Getenv("MYSQL_PASS")
}

func main() {
	db, _ = sql.Open("mysql", mysqlUser+":"+mysqlPass+"@tcp("+mysqlHost+":3306)/"+mysqlDb)
	defer db.Close()

	getMembersData()
	ticker := time.NewTicker(5 * time.Minute)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				getMembersData()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	close(quit)
	fmt.Println("Bye ;)")
}

func getMembersData() {
	members, err := cocapi.GetMemberInfo()
	if err != nil {
		reportError(err)
	}

	var ids = make([]string, 0)
	for _, m := range members.Items {
		if result, err := db.Exec(queryInsertUpdateMember, m.Tag, m.Name); err != nil {
			fmt.Println(err)
		} else {
			if id, err := result.LastInsertId(); err != nil {
				fmt.Println(err)
			} else {
				ids = append(ids, strconv.Itoa(int(id)))
			}
		}
	}
	db.Exec("UPDATE members SET active = 0 WHERE member_id NOT IN (" + strings.Join(ids, ", ") + ")")
	fmt.Println("done members func")
}

func reportError(err error) {
	fmt.Println(err)
	os.Exit(0)
}
