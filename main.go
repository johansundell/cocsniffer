package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var urlClan = "https://api.clashofclans.com/v1/clans/%s"
var urlMembers = "https://api.clashofclans.com/v1/clans/%s/members"
var myKey, myClanTag string
var db *sql.DB
var mysqlUser, mysqlPass, mysqlDb, mysqlHost string
var queryInsertUpdateMember = `INSERT INTO members (tag, name, created, last_updated, active) VALUES (?, ?, null, null, 1) ON DUPLICATE KEY UPDATE member_id=LAST_INSERT_ID(member_id), last_updated = NOW(), active = 1`

func init() {
	myKey = os.Getenv("COC_KEY")
	myClanTag = os.Getenv("COC_CLANTAG")
	mysqlDb = "cocsniffer"
	mysqlHost = "localhost"
	mysqlUser = os.Getenv("MYSQL_USER")
	mysqlPass = os.Getenv("MYSQL_PASS")
}

func main() {
	db, _ = sql.Open("mysql", mysqlUser+":"+mysqlPass+"@tcp("+mysqlHost+":3306)/"+mysqlDb)

	ticker := time.NewTicker(5 * time.Minute)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				getMembersData()
				fmt.Println("done members")
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
	members, err := getMemberInfo(myClanTag)
	if err != nil {
		reportError(err)
	}

	var ids = make([]string, 0)
	for _, m := range members.Items {
		//fmt.Println(m.Name, m.Tag)
		result, err := db.Exec(queryInsertUpdateMember, m.Tag, m.Name)
		if err != nil {
			fmt.Println(err)
		}
		id, err := result.LastInsertId()

		ids = append(ids, strconv.Itoa(int(id)))
	}
	//fmt.Println(strings.Join(ids, ", "))
	db.Exec("UPDATE members SET active = 0 WHERE member_id NOT IN (" + strings.Join(ids, ", ") + ")")
}

func reportError(err error) {
	fmt.Println(err)
	os.Exit(0)
}

type ClanInfo struct {
	BadgeUrls struct {
		Large  string `json:"large"`
		Medium string `json:"medium"`
		Small  string `json:"small"`
	} `json:"badgeUrls"`
	ClanLevel   int    `json:"clanLevel"`
	ClanPoints  int    `json:"clanPoints"`
	Description string `json:"description"`
	Location    struct {
		ID        int    `json:"id"`
		IsCountry bool   `json:"isCountry"`
		Name      string `json:"name"`
	} `json:"location"`
	MemberList []struct {
		ClanRank          int `json:"clanRank"`
		Donations         int `json:"donations"`
		DonationsReceived int `json:"donationsReceived"`
		ExpLevel          int `json:"expLevel"`
		League            struct {
			IconUrls struct {
				Medium string `json:"medium"`
				Small  string `json:"small"`
				Tiny   string `json:"tiny"`
			} `json:"iconUrls"`
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"league"`
		Name             string `json:"name"`
		PreviousClanRank int    `json:"previousClanRank"`
		Role             string `json:"role"`
		Trophies         int    `json:"trophies"`
	} `json:"memberList"`
	Members          int    `json:"members"`
	Name             string `json:"name"`
	RequiredTrophies int    `json:"requiredTrophies"`
	Tag              string `json:"tag"`
	Type             string `json:"type"`
	WarFrequency     string `json:"warFrequency"`
	WarWins          int    `json:"warWins"`
}

type Members struct {
	Items []struct {
		ClanRank          int `json:"clanRank"`
		Donations         int `json:"donations"`
		DonationsReceived int `json:"donationsReceived"`
		ExpLevel          int `json:"expLevel"`
		League            struct {
			IconUrls struct {
				Medium string `json:"medium"`
				Small  string `json:"small"`
				Tiny   string `json:"tiny"`
			} `json:"iconUrls"`
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"league"`
		Tag              string `json:"tag"`
		Name             string `json:"name"`
		PreviousClanRank int    `json:"previousClanRank"`
		Role             string `json:"role"`
		Trophies         int    `json:"trophies"`
	} `json:"items"`
}

func getMemberInfo(clanTag string) (members Members, err error) {
	body, err := getUrl(fmt.Sprintf(urlMembers, url.QueryEscape(clanTag)), myKey)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &members)
	return
}

func getClanInfo(clanTag string) (clan ClanInfo, err error) {
	body, err := getUrl(fmt.Sprintf(urlClan, url.QueryEscape(clanTag)), myKey)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &clan)
	return
}

func getUrl(url, key string) (b []byte, err error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Add("authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	b, err = ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		b = []byte{}
		err = errors.New("Error from server: " + strconv.Itoa(resp.StatusCode))
	}
	return
}
