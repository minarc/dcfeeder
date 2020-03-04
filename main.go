package main

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-redis/redis/v7"
)

type Post struct {
	number      uint32
	title       string
	description string
	thumbnail   string
	images      []string
	updated     string
}

type Pack struct {
	messages []Post
}

var hash = map[string]bool{}

func RequestList(url string) {
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	if res.StatusCode != 200 {
		log.Fatal(res.Status)
	}
	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		log.Fatal(err)
	}

	current := map[string]bool{}

	doc.Find(".gall_list > tbody").Children().Each(func(i int, s *goquery.Selection) {
		if dataType, exist := s.Attr("data-type"); exist && dataType != "icon_notice" {
			href, _ := s.Find(".gall_tit > a").Attr("href")
			current[href] = true
		}
	})

	var pack Pack = Pack{}
	for key := range current {
		if _, exist := hash[key]; !exist {
			test := RequestPost("http://gall.dcinside.com" + key)
			log.Println(test.title)
			pack.messages = append(pack.messages, test)
		}
	}

	hash = current

	go Publish(pack)
}

func RequestPost(url string) Post {
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Googlebot")

	httpClient := &http.Client{}
	res, err := httpClient.Do(req)

	if err != nil {
		log.Println(err)
	}
	if res.StatusCode != 200 {
		log.Println(res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		log.Println(err)
	}

	var message Post
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		op, _ := s.Attr("property")
		con, _ := s.Attr("content")

		if op == "og:image" {
			message.thumbnail = con
		} else if op == "og:title" {
			message.title = con
		} else if op == "og:description" {
			message.description = con
		} else if op == "og:updated_time" {
			message.updated = con
		}
	})

	re := regexp.MustCompile("dcimg[0-9]")
	doc.Find(".writing_view_box").Find("img").Each(func(i int, s *goquery.Selection) {
		url, _ := s.Attr("src")
		url = re.ReplaceAllString(url, "images")
		url = strings.Replace(url, "co.kr", "com", 1)

		message.images = append(message.images, url)
	})

	return message
}

func Publish(pack Pack) {
	message, _ := json.Marshal(pack.messages)
	log.Println(string(message))
	client.Publish("ib", message)
}

var client *redis.Client

func main() {
	client = redis.NewClient(&redis.Options{
		Addr:     "redis-10317.c16.us-east-1-3.ec2.cloud.redislabs.com:10317",
		Password: "WCkaZYzyhYR62p42VddCJba7Kn14vdvw",
		DB:       0,
	})

	if pong, err := client.Ping().Result(); err != nil {
		log.Fatal(err)
	} else {
		log.Println(pong)
	}

	for now := range time.Tick(time.Second * 5) {
		RequestList("https://gall.dcinside.com/board/lists?id=stream")
		log.Println("One cycle done", now)
	}
}
