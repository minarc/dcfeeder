package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcfee.com/model/proxies"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-redis/redis/v7"
)

type Post struct {
	Number      int                      `json:"number"`
	Title       string                   `json:"title"`
	Description string                   `json:"content"`
	Thumbnail   string                   `json:"thumbnail"`
	Images      []string                 `json:"images"`
	Updated     string                   `json:"updated"`
	Url         string                   `json:"url"`
	Vision      []map[string]interface{} `json:"vision"`
}

type Pack struct {
	Messages []Post `json:"result"`
}

var hash = map[string]int{}
var baseball = map[string]int{}
var pack *Pack

var proxyList []string

type LeastConnection struct {
}

type RoundRobin struct {
	Url  string
	Next *RoundRobin
}

func Make(proxies []string) {
	head := new(RoundRobin)
	cursor := head

	for _, p := range proxies {
		cursor = &RoundRobin{Url: p}
		cursor = cursor.Next
	}

	cursor.Next = head
}

func RequestBalancing(urls []string) {
	var wg sync.WaitGroup
	wg.Add(len(urls))

	for i, u := range urls {
		next, _ := url.Parse(proxyList[i])
		go RequestPost(u, 0, &http.Transport{Proxy: http.ProxyURL(next)}, &wg)
		next, _ = url.Parse(proxyList[i])
	}

	wg.Wait()
}

func RequestList(target string, hash *map[string]int, channel string) {
	req, err := http.NewRequest("GET", target, nil)
	req.Header.Set("User-Agent", "Googlebot")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", "gall.dcinside.com")
	req.Header.Set("Referer", "https://gall.dcinside.com")
	req.Header.Set("cookie", "PHPSESSID=08cfa4e74d0c71192a0895c9c1f8ec2c; ck_lately_gall=4RD%257C6Pn%257C5CY")

	httpClient := &http.Client{Timeout: time.Second * 1}
	res, err := httpClient.Do(req)

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

	pack = new(Pack)

	var targets []string
	for key := range current {
		if _, exist := (*hash)[key]; !exist {
			targets = append(targets, key)
		}
	}

	RequestBalancing(targets)

	*hash = current

	if len(pack.Messages) > 0 {
		Publish(pack, channel)
	}
}

func RequestPost(url string, number int, transport *http.Transport, wg *sync.WaitGroup) {
	defer wg.Done()

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Googlebot")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Host", "gall.dcinside.com")
	req.Header.Set("Referer", "https://gall.dcinside.com/board/lists?id=baseball_new8")
	req.Header.Set("cookie", "PHPSESSID=08cfa4e74d0c71192a0895c9c1f8ec2c; ck_lately_gall=4RD%257C6Pn%257C5CY")

	httpClient := &http.Client{Transport: transport, Timeout: time.Second * 3}

	startTime := time.Now()
	res, err := httpClient.Do(req)
	log.Println(number, time.Since(startTime))

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
	post.Url = url

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

func Visioning(encoded string, number int) []byte {
	payload := strings.NewReader(fmt.Sprintf(`{
		"instances":
		[
		  {
			"image_bytes":
			{
			  "b64": "%s"
			},
			"key": "%d"
		  }
		]
	  }`, encoded, number))

	req, err := http.NewRequest("POST", "http://localhost:8501/v1/models/default:predict", payload)
	if err != nil {
		return []byte("err")
	}

	req.Header.Add("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return []byte("err")
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return []byte("err")
	}

	return body
}

func Publish(pack *Pack, channel string) {
	message, _ := json.Marshal(pack)

	startTime := time.Now()
	client.Publish(channel, message)
	client.Set(channel, message, 0)
	log.Println(len(pack.Messages), "Message published", channel, time.Since(startTime))
}

var client *redis.Client

func main() {
	runtime.GOMAXPROCS(1)

	fpLog, err := os.OpenFile("logfile.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	defer fpLog.Close()

	multiWriter := io.MultiWriter(fpLog, os.Stdout)
	log.SetOutput(multiWriter)

	client = redis.NewClient(&redis.Options{
		// Addr: "seoul.arfrumo.codes:6379",
		// Addr: "34.64.196.220:6379",
		Addr: "127.0.0.1:6379",
		// Password: "WCkaZYzyhYR62p42VddCJba7Kn14vdvw",
		DB: 0,
	})

	if pong, err := client.Ping().Result(); err != nil {
		log.Fatal(err)
	} else {
		log.Println(pong)
	}

	proxies.UpdateProxyList()

	for now := range time.Tick(time.Second * 4) {
		RequestList("https://gall.dcinside.com/board/lists?id=baseball_new8", &baseball, "baseball")
		log.Println("One cycle done", now)
	}
}
