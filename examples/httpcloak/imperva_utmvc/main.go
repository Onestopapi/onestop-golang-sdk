// Imperva utmvc: the page loads the utmvc script from
// /_Incapsula_Resource?SWJIYLWA=<hash>. Load the script, store the generated
// value as the ___utmvc cookie, send the SWKMTFSR image request and load the
// page again.
package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strconv"

	"github.com/Onestopapi/onestop-golang-sdk/examples/httpcloak/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "YOUR_IDENTIFIER" // your identifier or account name
	pageURL        = "https://www.example.com/"
	acceptLanguage = "en-GB,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

var utmvcScript = regexp.MustCompile(`"([^"]*/_Incapsula_Resource\?SWJIYLWA=[^"]*)"`)

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page := getChallengePage(hc)
	scriptURL := findScript(page)
	session := newSession(client, ip, page.URL)
	script := getScript(hc, page, scriptURL)
	solveChallenge(hc, session, page, scriptURL, script)
	sendBeacon(hc, page)
	reloadPage(hc, page)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS ___utmvc cookie set, page re-GET 200")
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

func getChallengePage(hc *httpclient.Client) *httpclient.Response {
	return hc.Get(pageURL, httpclient.Navigate(acceptLanguage, ""))
}

// findScript returns the utmvc script the page loads from
// /_Incapsula_Resource?SWJIYLWA=<hash>.
func findScript(page *httpclient.Response) *url.URL {
	match := utmvcScript.FindStringSubmatch(page.Body)
	if match == nil {
		log.Fatalf("FAIL page returned %d without an Imperva utmvc script", page.Status)
	}
	return page.Resolve(match[1])
}

func getScript(hc *httpclient.Client, page *httpclient.Response, scriptURL *url.URL) string {
	res := hc.Get(scriptURL.String(), httpclient.Script(acceptLanguage, "same-origin", page.URL))
	if res.Status != 200 {
		log.Fatalf("FAIL script returned %d", res.Status)
	}
	return res.Body
}

func solveChallenge(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, scriptURL *url.URL, script string) {
	cookies := hc.Cookies(page.URL, "incap_ses_")
	res, err := session.ImpervaUtmvc(scriptURL.String(), script, cookies)
	if err != nil {
		log.Fatalf("FAIL imperva utmvc: %v", err)
	}
	// The script stores res.Payload as the ___utmvc cookie on the page host, path /.
	hc.SetCookie(page.Resolve("/").Hostname(), "___utmvc", res.Payload)
}

// sendBeacon is the image request the script makes after setting ___utmvc;
// e is Math.random().
func sendBeacon(hc *httpclient.Client, page *httpclient.Response) {
	beacon := page.Resolve("/_Incapsula_Resource")
	beacon.RawQuery = "SWKMTFSR=1&e=" + strconv.FormatFloat(rand.Float64(), 'f', -1, 64)
	hc.Get(beacon.String(), httpclient.Image(acceptLanguage, "", "same-origin", page.URL))
}

func reloadPage(hc *httpclient.Client, page *httpclient.Response) {
	loaded := hc.Get(page.URL, httpclient.Navigate(acceptLanguage, ""))
	if loaded.Status != 200 {
		log.Fatalf("FAIL page re-GET returned %d", loaded.Status)
	}
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
