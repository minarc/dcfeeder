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

	doc.Find(".gall_list > tbody").Children().Each(func(i int, s *goquery.Selection) {
		if dataType, exist := s.Attr("data-type"); exist && dataType != "icon_notice" {
			href, _ := s.Find(".gall_tit > a").Attr("href")

			if _, exist := hash[href]; exist {
				delete(hash, href)
			} else {
				hash[href] = true
			}
		}
	})

	var pack Pack
	for key := range hash {
		test := RequestPost("http://gall.dcinside.com" + key)
		log.Println(test.title)
		pack.messages = append(pack.messages, test)
	}

	Publish(pack)
}

func RequestPost(url string) Post {
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	if res.StatusCode != 200 {
		log.Fatal(res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		log.Fatal(err)
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
	message, _ := json.Marshal(pack)
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
	pong, err := client.Ping().Result()
	log.Println(pong, err)

	for now := range time.Tick(time.Second * 10) {
		RequestList("https://gall.dcinside.com/board/lists?id=baseball_new8")
		log.Println("One cycle done", now)
	}
}
