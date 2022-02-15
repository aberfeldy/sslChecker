package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type Check struct {
	domain string
	valid  bool
	expire string
}
type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}
type Attachement struct {
	MrkdwnIn []string `json:"mrkdwn_in"`
	Color    string   `json:"color"`
	Pretext  string   `json:"pretext"`
	Text     string   `json:"text"`
	Fields   []Field  `json:"fields"`
	Ts       int64    `json:"ts"`
}
type SlackMessage struct {
	Attachments []Attachement `json:"attachments"`
}

var (
	webhook    string
	configPath string
	domains    chan string
	done       chan bool
	checks     []Check
)

func init() {
	wh := os.Getenv("SLACK_WEBHOOK")
	if wh == "" {
		log.Fatal("env SLACK_WEBHOOK not set")
	}
	webhook = wh

	cp := os.Getenv("CONFIG")
	if cp == "" {
		log.Fatal("env CONFIG not set")
	}
	configPath = cp

	domains = make(chan string)
	done = make(chan bool)
	checks = make([]Check, 0)
}

func main() {
	go read()
	for i := 0; i < 2; i++ {
		go compute(domains)
	}
	<-done
	err := SendSlackNotification(checks)
	if err != nil {
		log.Fatal(err)
	}
}
func read() {
	file, err := os.Open(configPath + "domains.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		domains <- scanner.Text()
	}
	close(domains)
}
func compute(queue chan string) {
	for line := range queue {
		checks = append(checks, checkExpiry(line))
	}
	done <- true
}

func checkExpiry(domain string) Check {
	check := Check{domain: domain, valid: false, expire: ""}

	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:443", domain), nil)
	if err != nil {
		log.Println("Server doesn't support SSL certificate err: " + err.Error())
		recover()
		check.expire = err.Error()
		return check
	}
	expiry := conn.ConnectionState().PeerCertificates[0].NotAfter
	now := time.Now()
	diff := expiry.Sub(now).Hours()
	check.expire = expiry.Format(time.RFC850)

	if diff > 168 {
		check.valid = true
	}
	return check
}

func SendSlackNotification(checks []Check) error {
	var fields []Field
	fields = make([]Field, 0)
	for _, v := range checks {
		if !v.valid {
			fields = append(fields, Field{
				Title: v.domain,
				Value: v.expire,
				Short: false,
			})
		}
	}
	if len(fields) < 1 {
		return nil
	}
	msg := SlackMessage{
		Attachments: []Attachement{
			{
				MrkdwnIn: []string{"text"},
				Color:    "danger",
				Pretext:  "<!channel> SSL Checker found following Certs expiring in 7 days or with errors",
				Fields:   fields,
				Ts:       time.Now().Unix(),
			},
		},
	}

	slackBody, _ := json.Marshal(msg)
	req, err := http.NewRequest(http.MethodPost, webhook, bytes.NewBuffer(slackBody))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if buf.String() != "ok" {
		return errors.New("non-ok response returned from Slack")
	}

	return nil
}
