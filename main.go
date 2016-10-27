package main

import (
	"database/sql"
	"flag"
	"log"
	"log/syslog"
	"net/smtp"
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
var isCocUnderUpdate bool
var failedTries int

func init() {

	mysqlDb = "cocsniffer"
	mysqlHost = os.Getenv("MYSQL_COC_HOST")
	mysqlUser = os.Getenv("MYSQL_USER")
	mysqlPass = os.Getenv("MYSQL_PASS")
}

func main() {
	useSyslog := flag.Bool("syslog", false, "Use syslog")
	flag.Parse()
	if *useSyslog {
		logwriter, e := syslog.New(syslog.LOG_NOTICE, "cocsniffer")
		if e == nil {
			log.SetOutput(logwriter)
		}
	}
	db, _ = sql.Open("mysql", mysqlUser+":"+mysqlPass+"@tcp("+mysqlHost+":3306)/"+mysqlDb)
	defer db.Close()

	isCocUnderUpdate = false
	failedTries = 0
	getMembersData()
	ticker := time.NewTicker(1 * time.Minute)
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

	// Wait for SIGINT and SIGTERM (HIT CTRL-C)
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	close(quit)
	log.Println("Bye ;)")
}

func getMembersData() {
	members, err := cocapi.GetMemberInfo()
	if err != nil {
		reportError(err)
		return
	}

	if isCocUnderUpdate {
		isCocUnderUpdate = false
		sendEmail("johan@pixpro.net", "johan@sundell.com", "COC Alert", "Servers are up again")
	}
	failedTries = 0

	var ids = make([]string, 0)
	for _, m := range members.Items {
		if result, err := db.Exec(queryInsertUpdateMember, m.Tag, m.Name); err != nil {
			log.Println(err)
		} else {
			if id, err := result.LastInsertId(); err != nil {
				log.Println(err)
			} else {
				ids = append(ids, strconv.Itoa(int(id)))
			}
		}
	}
	db.Exec("UPDATE members SET exited = NOW() WHERE member_id NOT IN (" + strings.Join(ids, ", ") + ") AND active = 1")
	db.Exec("UPDATE members SET active = 0 WHERE member_id NOT IN (" + strings.Join(ids, ", ") + ")")
	log.Println("done members func")
}

func reportError(err error) {
	switch t := err.(type) {
	case *cocapi.ServerError:
		if t.ErrorCode == 503 {
			failedTries++
			if failedTries > 3 {
				if !isCocUnderUpdate {
					isCocUnderUpdate = true
					sendEmail("johan@pixpro.net", "johan@sundell.com", "COC Alert", "Servers under update")
				}
			}
		}
		break
	default:
		log.Println("Fatal error coc:", t)
		break
	}
}

func sendEmail(to, from, subject, message string) bool {
	body := "To: " + to + "\r\nSubject: " + subject + "\r\n\r\n" + message
	if err := smtp.SendMail("127.0.0.1:25", nil, from, []string{to}, []byte(body)); err != nil {
		log.Println(err)
		return false
	}
	return true
}
