package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
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
	Url         string   `json:"url"`
	Vision      []string `json:"vision"`
}

type Pack struct {
	Messages []Post `json:"result"`
}

var hash = map[string]int{}
var baseball = map[string]int{}
var pack *Pack

func RequestList(url string, hash *map[string]int, channel string) {
	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Googlebot")
	req.Header.Set("cookie", "PHPSESSID=08cfa4e74d0c71192a0895c9c1f8ec2c; ck_lately_gall=4RD%257C6Pn%257C5CY")

	httpClient := &http.Client{Timeout: time.Second * 5}
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

	var wg sync.WaitGroup
	pack = new(Pack)

	limit := 0
	for key, number := range current {
		if _, exist := (*hash)[key]; !exist && limit < 10 {
			wg.Add(1)
			limit++
			go RequestPost("https://gall.dcinside.com"+key, number, &wg)
			time.Sleep(time.Millisecond * 200)
		}
	}

	wg.Wait()
	*hash = current

	if len(pack.Messages) > 0 {
		Publish(pack, channel)
	}
}

func RequestPost(url string, number int, wg *sync.WaitGroup) {
	defer wg.Done()

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Googlebot")
	req.Header.Set("cookie", "PHPSESSID=08cfa4e74d0c71192a0895c9c1f8ec2c; ck_lately_gall=4RD%257C6Pn%257C5CY")

	httpClient := &http.Client{Timeout: time.Second * 5}

	// startTime := time.Now()
	res, err := httpClient.Do(req)
	// log.Println(number, time.Since(startTime))

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

func Visioning(encoded string, number int) string {
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
		return err.Error()
	}

	req.Header.Add("content-type", "application/json")

	startTime := time.Now()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err.Error()
	}
	log.Println("Model predicted", string(body), time.Since(startTime))

	return string(body)
}

func GetBase64FromURL(url string) string {
	startTime := time.Now()

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Googlebot")
	req.Header.Set("cookie", "PHPSESSID=08cfa4e74d0c71192a0895c9c1f8ec2c; ck_lately_gall=4RD%257C6Pn%257C5CY")

	httpClient := &http.Client{Timeout: time.Second * 1}

	res, err := httpClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err.Error()
	}

	log.Println("Got base64 from url", time.Since(startTime))

	return b64.StdEncoding.EncodeToString(body)
}

func Publish(pack *Pack, channel string) {

	for i := range pack.Messages {
		if len(pack.Messages[i].Images) > 0 {
			for _, url := range pack.Messages[i].Images {
				pack.Messages[i].Vision = append(pack.Messages[i].Vision, Visioning(GetBase64FromURL(url), pack.Messages[i].Number))
				time.Sleep(time.Millisecond * 100)
			}
		}
	}

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

	go func() {
		log.Println(http.ListenAndServe("127.0.0.1:6060", nil))
	}()

	client = redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "WCkaZYzyhYR62p42VddCJba7Kn14vdvw",
		DB:       0,
	})

	if pong, err := client.Ping().Result(); err != nil {
		log.Fatal(err)
	} else {
		log.Println(pong)
	}

	// galleries := []string{"https://gall.dcinside.com/board/lists?id=stream", "https://gall.dcinside.com/board/lists?id=baseball_new8"}

	for now := range time.Tick(time.Second * 4) {

		RequestList("https://gall.dcinside.com/board/lists?id=stream", &hash, "streamer")
		RequestList("https://gall.dcinside.com/board/lists?id=baseball_new8", &baseball, "baseball")
		log.Println("One cycle done", now)
	}
}
