package proxies

type Proxy struct {
	Host string
	Port int
}

func (p *Proxy) getRandomProxy() {

}

func (p *Proxy) init() {
	// file, _ := ioutil.ReadFile("public/proxies.yaml")
}

func (p Proxy) AvailableProxies() {

}

func UpdateProxyList() {
	// file, _ := ioutil.ReadFile("public/proxies.yaml")

	// var temp map[string]interface{}
	// yaml.Unmarshal(file, &temp)

	// log.Println(temp)
}
