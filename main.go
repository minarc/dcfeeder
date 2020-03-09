package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-redis/redis/v7"
)

type Post struct {
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	Description string   `json:"content"`
	Thumbnail   string   `json:"thumbnail"`
	Images      []string `json:"images"`
	Updated     string   `json:"updated"`
}

type Pack struct {
	Messages []Post `json:"result"`
}

var hash = map[string]int{}
var baseball = map[string]int{}
var pack *Pack

func RequestList(url string, hash *map[string]int, channel string) {
	res, err := http.Get(url)

	if err != nil {
		log.Println(err)
		return
	}

	if res.StatusCode != 200 {
		log.Println(res.Status)
		return
	}

	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		log.Println(err)
		return
	}

	current := map[string]int{}
	doc.Find(".gall_list > tbody").Children().Each(func(i int, s *goquery.Selection) {
		if dataType, exist := s.Attr("data-type"); exist && dataType != "icon_notice" {
			href, _ := s.Find(".gall_tit > a").Attr("href")
			number, _ := strconv.Atoi(s.Find(".gall_num").Text())
			current[href] = number
		}
	})

	var wg sync.WaitGroup

	pack = new(Pack)
	for key, number := range current {
		if _, exist := (*hash)[key]; !exist {
			wg.Add(1)
			go RequestPost("http://gall.dcinside.com"+key, number, &wg)
			time.Sleep(time.Millisecond * 250)
		}
	}

	wg.Wait()
	*hash = current

	if len(pack.Messages) > 0 {
		go Publish(pack, channel)
	}
}

func RequestPost(url string, number int, wg *sync.WaitGroup) {
	defer wg.Done()

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Googlebot")

	httpClient := &http.Client{Timeout: time.Second * 5}
	res, err := httpClient.Do(req)

	if err != nil {
		log.Println(err)
		return
	}

	if res.StatusCode != 200 {
		log.Println(res.StatusCode, res.Status)
		return
	}

	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		log.Println(err)
		return
	}

	post := new(Post)
	post.Number = number

	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		op, exist := s.Attr("property")
		if !exist {
			return
		}
		con, exist := s.Attr("content")
		if !exist {
			return
		}

		if op == "og:image" {
			post.Thumbnail = strings.Replace(con, "write", "images", 1)
		} else if op == "og:title" {
			splited := strings.Split(con, "-")
			title := strings.Join(splited[:1], "")
			post.Title = strings.TrimSpace(title)
		} else if op == "og:description" {
			if strings.HasPrefix(con, "국내 최대") {
				con = ""
			}
			post.Description = con
		} else if op == "og:updated_time" {
			post.Updated = con
		}
	})

	re := regexp.MustCompile("dcimg[0-9]")
	doc.Find(".writing_view_box").Find("img").Each(func(i int, s *goquery.Selection) {
		url, exist := s.Attr("src")
		if !exist {
			return
		}
		url = re.ReplaceAllString(url, "images")
		url = strings.Replace(url, "co.kr", "com", 1)

		post.Images = append(post.Images, url)
	})

	pack.Messages = append(pack.Messages, *post)
}

func Publish(pack *Pack, channel string) {
	// message, _ := json.Marshal(pack)
	// client.Publish(channel, message)
	// client.Set(channel, message, 0)
	log.Println(len(pack.Messages), "Message published", channel)
}

var client *redis.Client

func main() {
	runtime.GOMAXPROCS(1)

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

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
		RequestList("https://gall.dcinside.com/board/lists?id=stream", &hash, "streamer")
		RequestList("https://gall.dcinside.com/board/lists?id=baseball_new8", &baseball, "baseball")
		log.Println("One cycle done", now)
	}
}
