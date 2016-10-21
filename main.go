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

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	_ "github.com/go-sql-driver/mysql"
	"github.com/johansundell/cocapi"
)

var db *sql.DB
var mysqlUser, mysqlPass, mysqlDb, mysqlHost string
var queryInsertUpdateMember = `INSERT INTO members (tag, name, created, last_updated, active) VALUES (?, ?, null, null, 1) ON DUPLICATE KEY UPDATE member_id=LAST_INSERT_ID(member_id), last_updated = NOW(), active = 1`
var consumerKey, consumerSecret, accessToken, accessSecret string

func init() {

	mysqlDb = "cocsniffer"
	mysqlHost = os.Getenv("MYSQL_COC_HOST")
	mysqlUser = os.Getenv("MYSQL_USER")
	mysqlPass = os.Getenv("MYSQL_PASS")

	consumerKey = os.Getenv("TWITTER_CONSKEY")
	consumerSecret = os.Getenv("TWITTER_CONSSEC")
	accessToken = os.Getenv("TWITTER_ACCTOK")
	accessSecret = os.Getenv("TWITTER_ACCSEC")
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

	config := oauth1.NewConfig(consumerKey, consumerSecret)
	token := oauth1.NewToken(accessToken, accessSecret)
	// OAuth1 http.Client will automatically authorize Requests
	httpClient := config.Client(oauth1.NoContext, token)

	// Twitter client
	client := twitter.NewClient(httpClient)

	// Convenience Demux demultiplexed stream messages
	demux := twitter.NewSwitchDemux()
	demux.Tweet = func(tweet *twitter.Tweet) {
		log.Println("found one", tweet.Text)
		log.Println(tweet.User.ScreenName)
		if strings.Contains(strings.ToLower(tweet.Text), strings.ToLower("Maintenance")) && (tweet.User.ID == 730400376 || tweet.User.ID == 250293507) {
			sendEmail("johan@pixpro.net", "johan@sundell.com", "COC alert", tweet.Text)
			log.Println("Email sent:", tweet.Text)
		}
	}
	demux.DM = func(dm *twitter.DirectMessage) {
		//fmt.Println(dm.SenderID)
	}
	demux.Event = func(event *twitter.Event) {
		//fmt.Printf("%#v\n", event)
	}

	log.Println("Starting Stream...")

	// FILTER
	filterParams := &twitter.StreamFilterParams{
		Follow: []string{"730400376", "240359880", "250293507"},
		//Track:         []string{"Maintenance", "Maintenance.", "sudde"},
		StallWarnings: twitter.Bool(true),
	}

	stream, err := client.Streams.Filter(filterParams)
	if err != nil {
		log.Fatal(err)
	}

	// Receive messages until stopped or stream quits
	go demux.HandleChan(stream.Messages)

	// Wait for SIGINT and SIGTERM (HIT CTRL-C)
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	log.Println(<-ch)

	close(quit)
	stream.Stop()
	log.Println("Bye ;)")
}

func getMembersData() {
	members, err := cocapi.GetMemberInfo()
	if err != nil {
		reportError(err)
	}

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
	db.Exec("UPDATE members SET active = 0 WHERE member_id NOT IN (" + strings.Join(ids, ", ") + ")")
	log.Println("done members func")
}

func reportError(err error) {
	log.Println("Fatal error:", err)
	os.Exit(0)
}

func sendEmail(to, from, subject, message string) bool {
	body := "To: " + to + "\r\nSubject: " + subject + "\r\n\r\n" + message
	if err := smtp.SendMail("127.0.0.1:25", nil, from, []string{to}, []byte(body)); err != nil {
		log.Println(err)
		return false
	}
	return true
}
