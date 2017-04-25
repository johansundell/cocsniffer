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
var emailTo, emailFrom string
var myClanTag, myKey string
var cocClient cocapi.Client

func init() {

	mysqlDb = "cocsniffer"
	mysqlHost = os.Getenv("MYSQL_COC_HOST")
	mysqlUser = os.Getenv("MYSQL_USER")
	mysqlPass = os.Getenv("MYSQL_PASS")

	emailTo = os.Getenv("EMAIL_TO")
	emailFrom = os.Getenv("EMAIL_FROM")

	myClanTag = os.Getenv("COC_CLANTAG")
	myKey = os.Getenv("COC_KEY")
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

	cocClient = cocapi.NewClient(myKey)

	isCocUnderUpdate = false
	failedTries = 0
	getMembersData(myClanTag)
	//return
	ticker := time.NewTicker(10 * time.Second)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				getMembersData(myClanTag)
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

func getPlayerInfo() error {
	rows, err := db.Query("SELECT tag FROM members WHERE active = 1")
	if err != nil {
		return err
	}

	for rows.Next() {
		var tag string
		err := rows.Scan(&tag)
		if err != nil {
			log.Println(err)
			continue
		}
		player, err := cocClient.GetPlayerInfo(tag)
		if err != nil {
			log.Println(err)
			continue
		}
		if _, err := db.Exec("UPDATE members SET war_stars = ? WHERE tag = ?", player.WarStars, tag); err != nil {
			log.Println(err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}

func getMembersData(clan string) error {
	members, err := cocClient.GetMembers(clan)
	if err != nil {
		//log.Println(err)
		reportError(err)
		return err
	}

	if isCocUnderUpdate {
		isCocUnderUpdate = false
		sendEmail("COC Alert", "Servers are up again")
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
				donations := 0
				if err := db.QueryRow("SELECT current_donations FROM members WHERE member_id = ?", id).Scan(&donations); err != nil {
					log.Println(err)
				} else {
					//log.Println(m.Donations, donations)
					if m.Donations != donations {
						if _, err := db.Exec("UPDATE members SET prev_donations = ?, current_donations = ?, last_donation_time = NOW() WHERE member_id = ?", donations, m.Donations, id); err != nil {
							log.Println(err)
						}
						if m.Donations > donations {
							if _, err := db.Exec("INSERT donations (member_id, ts, current_donations, prev_donations) VALUES (?, NOW(), ?, ?)", id, m.Donations, donations); err != nil {
								log.Println(err)
							}
						}
					}
				}
				ids = append(ids, strconv.Itoa(int(id)))
			}
		}
		if m.Role == "member" && m.Donations >= 1000 {
			log.Println("Found member that should be upgraded", m.Name)
			var alerted int
			db.QueryRow("SELECT alert_sent_donations FROM members WHERE tag = ?", m.Tag).Scan(&alerted)
			if alerted == 0 {
				sendEmail("Member "+m.Name+" should be upgraded", "Member "+m.Name+" should be upgraded")
				db.Exec("UPDATE members SET alert_sent_donations = 1 WHERE tag = ?", m.Tag)
			}
		}
	}
	db.Exec("UPDATE members SET exited = NOW() WHERE member_id NOT IN (" + strings.Join(ids, ", ") + ") AND active = 1")
	db.Exec("UPDATE members SET active = 0 WHERE member_id NOT IN (" + strings.Join(ids, ", ") + ")")
	//log.Println("done members func")
	return nil
}

func reportError(err error) {
	switch t := err.(type) {
	case *cocapi.ServerError:
		if t.ErrorCode == 503 {
			failedTries++
			if failedTries > 3 {
				if !isCocUnderUpdate {
					isCocUnderUpdate = true
					sendEmail("COC Alert", "Servers under update")
				}
			}
		}
		break
	default:
		log.Println("Fatal error coc:", t)
		break
	}
}

func sendEmail(subject, message string) bool {
	body := "To: " + emailTo + "\r\nSubject: " + subject + "\r\n\r\n" + message
	if err := smtp.SendMail("127.0.0.1:25", nil, emailFrom, []string{emailTo}, []byte(body)); err != nil {
		log.Println(err)
		return false
	}
	return true
}
