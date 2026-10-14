// Kasada: load the page and its p.js, then mfc, fp (429) and ips.js, and
// POST the generated /tl body. A page that is already the 429 block page
// links ips.js itself. /tl answers 200 with reload:true.
package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/Onestopapi/onestop-golang-sdk/examples/httpcloak/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "" // your identifier or account name
	pageURL        = "https://www.example.com/"
	acceptLanguage = "en-US,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

var (
	pjsPath   = regexp.MustCompile(`/[0-9a-f-]{36}/[0-9a-f-]{36}/p\.js`)
	ipsScript = regexp.MustCompile(`src="([^"]*/ips\.js[^"]*)"`)
)

// ips is the loaded ips.js: its URL, its source and the page that loaded it.
type ips struct {
	url, source, referer string
}

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	session := newSession(client, ip, pageURL)
	page := getPage(hc)
	script := getIps(hc, session, page)
	postTl(hc, session, page, script)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS /tl 200 reload:true")
}

func newClient() (*onestop.Client, string) {
	client, err := onestop.New(onestop.Config{
		APIKey:       apiKey,
		Identifier:   identifier,
		IPHTTPClient: httpclient.IPClient(proxy),
	})
	if err != nil {
		log.Fatalf("FAIL %v", err)
	}
	ip, err := client.GetIP() // through the proxy: the IP the site sees
	if err != nil {
		log.Fatalf("FAIL %v", err)
	}
	return client, ip
}

func newSession(client *onestop.Client, ip, site string) onestop.Session {
	return client.Session(onestop.Session{
		Site:           site,
		Referer:        site,
		UserAgent:      httpclient.UserAgent,
		AcceptLanguage: acceptLanguage,
		IP:             ip,
	})
}

func getPage(hc *httpclient.Client) *httpclient.Response {
	return hc.Get(pageURL, httpclient.Navigate(acceptLanguage, ""))
}

// getIps loads ips.js. When the page itself is the 429 block page it links
// ips.js directly; otherwise p.js, mfc and fp lead to it.
func getIps(hc *httpclient.Client, session onestop.Session, page *httpclient.Response) ips {
	from := page
	if page.Status != 429 || !ipsScript.MatchString(page.Body) {
		base, version := getPScript(hc, session, page)
		from = getFp(hc, page, base, version)
	}
	ipsURL := ipsURLFrom(from)
	res := hc.Get(ipsURL, httpclient.Script(acceptLanguage, "same-origin", from.URL))
	if res.Status != 200 {
		log.Fatalf("FAIL ips.js returned %d", res.Status)
	}
	return ips{ipsURL, res.Body, from.URL}
}

// getPScript returns the script directory and x-kpsdk-v from the p.js result.
func getPScript(hc *httpclient.Client, session onestop.Session, page *httpclient.Response) (base, version string) {
	// p.js lives at /<uuid>/<uuid>/p.js; mfc, fp, ips.js and tl sit next to it.
	// Some pages write the path JSON-escaped (\/ or \u002f for /).
	path := pjsPath.FindString(strings.NewReplacer(`\/`, "/", `\u002f`, "/").Replace(page.Body))
	if path == "" {
		log.Fatalf("FAIL page returned %d without a Kasada p.js", page.Status)
	}
	u, _ := url.Parse(page.URL)
	base = u.Scheme + "://" + u.Host + strings.TrimSuffix(path, "p.js")
	pjs := hc.Get(base+"p.js", httpclient.Script(acceptLanguage, "same-origin", page.URL))
	ps, err := session.KasadaPScript(pjs.Body)
	if err != nil {
		log.Fatalf("FAIL p.js: %v", err)
	}
	return base, ps.Headers["x-kpsdk-v"]
}

// getFp loads mfc and fp. fp answers 429 with a body that links ips.js.
func getFp(hc *httpclient.Client, page *httpclient.Response, base, version string) *httpclient.Response {
	mfc := hc.Get(base+"mfc", httpclient.KasadaMfc(acceptLanguage, version, page.URL))
	if mfc.Status != 200 {
		log.Fatalf("FAIL mfc returned %d", mfc.Status)
	}
	fp := hc.Get(base+"fp?x-kpsdk-v="+version, httpclient.Frame(acceptLanguage, "same-origin", page.URL))
	if !ipsScript.MatchString(fp.Body) {
		log.Fatal("FAIL fp returned no ips.js")
	}
	return fp
}

// ipsURLFrom returns the ips.js src a Kasada 429 body links, HTML-unescaped
// and resolved against the response URL.
func ipsURLFrom(res *httpclient.Response) string {
	return res.Resolve(ipsScript.FindStringSubmatch(res.Body)[1]).String()
}

func postTl(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, script ips) {
	tl, err := session.KasadaTL(script.url, script.source)
	if err != nil {
		log.Fatalf("FAIL tl: %v", err)
	}
	u, _ := url.Parse(page.URL)
	origin := u.Scheme + "://" + u.Host
	base, _, _ := strings.Cut(script.url, "ips.js")
	body, _ := base64.StdEncoding.DecodeString(tl.Payload) // Payload is the /tl body in base64
	res := hc.Post(base+"tl", httpclient.KasadaTL(acceptLanguage, tl.Headers, origin, script.referer), string(body))
	if res.Status != 200 || !strings.Contains(res.Body, `"reload":true`) {
		log.Fatalf("FAIL /tl returned %d: %s", res.Status, res.Body)
	}
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
