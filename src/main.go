package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	proxies "dcfeed/model"

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

func (p Pack) Len() int {
	return len(p.Messages)
}

func (p Pack) Swap(i, j int) {
	p.Messages[i], p.Messages[j] = p.Messages[j], p.Messages[i]
}

func (p Pack) Less(i, j int) bool {
	return p.Messages[i].Number < p.Messages[j].Number
}

var hash = map[string]int{}
var baseball = map[string]int{}
var pack *Pack

var proxy []proxies.Server
var round *Node

var httpConnectionPool *http.Client

type Node struct {
	Server proxies.Server
	Next   *Node
}

func ProxyLinkedList(servers []proxies.Server) *Node {
	var head *Node = new(Node)
	var cursor *Node = head

	for _, p := range servers {
		cursor.Server = p
		cursor.Next = new(Node)

		cursor = cursor.Next
	}

	cursor.Server = proxies.Server{Transport: http.DefaultTransport, Location: "서울 GCP"}
	cursor.Next = head

	return head
}

func RequestBalancing(urls []string) {
	var wg sync.WaitGroup
	wg.Add(len(urls))

	start := time.Now()
	for i, u := range urls {
		if parsed, err := url.ParseQuery(strings.Split(u, "?")[1]); err != nil {
			log.Println(err.Error())
		} else {
			gallery := parsed["id"][0]
			number := parsed["no"][0]

			go RequestPost(fmt.Sprintf("https://m.dcinside.com/board/%s/%s", gallery, number), i, &round.Server, &wg)
			round = round.Next
		}
	}

	wg.Wait()
	log.Println(time.Since(start))
}

func RequestList(target string, hash *map[string]int, channel string) {
	req, err := http.NewRequest("GET", target, nil)
	req.Header.Set("User-Agent", "Googlebot")
	req.Header.Set("Host", "gall.dcinside.com")
	req.Header.Set("Referer", "https://gall.dcinside.com")

	res, err := httpConnectionPool.Do(req)

	if err != nil {
		log.Println(err)
		return
	}

	if res.StatusCode != 200 {
		log.Println(res.Status)
		return
	}

	defer res.Body.Close()

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

	if len(targets) > 10 {
		targets = targets[:10]
	}

	RequestBalancing(targets)

	*hash = current

	if len(pack.Messages) > 0 {
		go Publish(float32(len(targets)), pack, channel)
	}
}

func RequestPost(url string, number int, server *proxies.Server, wg *sync.WaitGroup) {
	defer wg.Done()

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; U; Android 4.4.2; en-us; SCH-I535 Build/KOT49H) AppleWebKit/534.30 (KHTML, like Gecko) Version/4.0 Mobile Safari/534.30")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Referer", "https://m.dcinside.com")

	counter := 0

RETRY:
	httpClient := &http.Client{Transport: server.Transport, Timeout: time.Millisecond * 400}

	startTime := time.Now()
	res, err := httpClient.Do(req)

	if err != nil {
		server.Failed++
		counter++
		if counter > 3 {
			log.Println(server.Location, "Failed")
			return
		}
		goto RETRY
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Println(res.StatusCode, res.Status)
		server.Failed++
		return
	}

	server.Success++
	log.Println(server.Location, float32(server.Success)/float32(server.Success+server.Failed), time.Since(startTime))

	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		log.Println(err)
		return
	}

	post := new(Post)
	post.Number = number
	post.Url = url
	post.Title = strings.Split(doc.Find("title").Text(), "-")[0]

	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		name, exist := s.Attr("name")
		if !exist {
			return
		}
		content, exist := s.Attr("content")
		if !exist {
			return
		}

		if name == "og:image" {
			post.Thumbnail = strings.Replace(content, "write", "images", 1)
		} else if name == "description" {
			if strings.HasPrefix(content, "국내 최대") {
				content = ""
			}
			post.Description = content
		} else if name == "og:updated_time" {
			post.Updated = content
		}
	})

	re := regexp.MustCompile("dcimg[0-9]")
	doc.Find(".thum-txtin").Find("img").Each(func(i int, s *goquery.Selection) {
		url, exist := s.Attr("data-original")
		if !exist {
			return
		}
		url = re.ReplaceAllString(url, "images")
		url = strings.Replace(url, "co.kr", "com", 1)

		post.Images = append(post.Images, url)
	})

	if len(post.Images) > 0 {
		post.Thumbnail = post.Images[0]
	}

	pack.Messages = append(pack.Messages, *post)
}

func Publish(target float32, pack *Pack, channel string) {
	startTime := time.Now()
	sort.Sort(pack)

	if message, err := json.Marshal(pack); err != nil {
		log.Println(err.Error())
	} else {
		client.Publish(channel, message)

		for _, post := range pack.Messages {
			if p, err := json.Marshal(post); err != nil {
				log.Println(err.Error())
			} else {
				client.LPush(channel, p)
				client.LTrim(channel, 0, 9)
			}
		}

		log.Println("Message published", channel, time.Since(startTime), target, float32(len(pack.Messages))/target*100)
	}
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
		Addr:     "127.0.0.1:6379",
		Password: "WCkaZYzyhYR62p42VddCJba7Kn14vdvw",
		DB:       0,
	})

	if pong, err := client.Ping().Result(); err != nil {
		log.Fatal(err)
	} else {
		log.Println(pong)
	}

	round = ProxyLinkedList(proxies.UpdateServerList())

	defaultTransportPointer, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		panic(fmt.Sprintf("defaultRoundTripper not an *http.Transport"))
	}
	defaultTransport := *defaultTransportPointer
	defaultTransport.MaxIdleConns = 5
	defaultTransport.MaxIdleConnsPerHost = 5
	httpConnectionPool = &http.Client{Transport: &defaultTransport, Timeout: time.Second}

	for range time.Tick(time.Second * 3) {
		RequestList("https://gall.dcinside.com/board/lists?id=baseball_new8", &baseball, "baseball")
		log.Println("One cycle done")
	}
}
