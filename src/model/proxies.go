package proxies

import (
	"io/ioutil"
	"log"
	"net/url"

	"gopkg.in/yaml.v2"
)

type Proxy struct {
	Url      *url.URL
	Location string
}

func (p *Proxy) getRandomProxy() {

}

func (p *Proxy) init() {
	// file, _ := ioutil.ReadFile("public/proxies.yaml")
}

func (p Proxy) AvailableProxies() {

}

func UpdateProxyList() []Proxy {
	var result []Proxy

	if file, err := ioutil.ReadFile("../public/proxies.yaml"); err != nil {
		log.Fatal(err)
	} else {
		proxies := make(map[interface{}][]map[string]string)
		if err := yaml.Unmarshal(file, &proxies); err != nil {
			log.Fatal(err)
		}

		for _, p := range proxies["korea"] {
			if url, err := url.Parse(p["host"]); err != nil {
				log.Fatal(err)
			} else {
				result = append(result, Proxy{Url: url, Location: p["location"]})
			}
		}
	}
	return result
}
