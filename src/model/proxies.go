package proxies

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	_ "github.com/Bogdan-D/go-socks4"
	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v2"
)

type Server struct {
	Transport http.RoundTripper
	Url       *url.URL
	Type      string
	Location  string
	Success   int
	Failed    int
	Latency   time.Duration
}

func UpdateServerList() []Server {
	var result []Server

	if file, err := ioutil.ReadFile("../public/proxies.yaml"); err != nil {
		log.Fatal(err)
	} else {
		proxies := make(map[interface{}][]map[string]string)
		if err := yaml.Unmarshal(file, &proxies); err != nil {
			log.Fatal(err)
		}

		for _, p := range proxies["korea"] {
			if p["protocol"] == "http" || p["protocol"] == "https" {
				if url, err := url.Parse(p["host"]); err != nil {
					log.Fatal(err)
				} else {
					result = append(result, Server{Transport: &http.Transport{Proxy: http.ProxyURL(url)}, Url: url, Location: p["location"]})
				}
			} else if p["protocol"] == "socks5" {
				if dialer, err := proxy.SOCKS5("tcp", p["host"], nil, proxy.Direct); err != nil {
					log.Fatal(err)
				} else {
					result = append(result, Server{Transport: &http.Transport{Dial: dialer.Dial}, Location: p["location"]})
				}
			} else if p["protocol"] == "socks4" {
				if url, err := url.Parse("socks4://" + p["host"]); err != nil {
					log.Fatal(err)
				} else {
					dialer, _ := proxy.FromURL(url, proxy.Direct)
					result = append(result, Server{Transport: &http.Transport{Dial: dialer.Dial}, Location: p["location"]})
				}
			}
		}
	}
	return result
}
