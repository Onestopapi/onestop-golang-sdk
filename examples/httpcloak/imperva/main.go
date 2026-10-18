// Imperva: the page answers with the reese84 challenge page, which adds the
// challenge script. Load the script, POST the generated body to the script's
// path with ?d=<host>, store the returned token as the reese84 cookie and
// load the page again.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"

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

var challengeScript = regexp.MustCompile(`scriptElement\.src\s*=\s*"([^"]+)"`)

// tokenReply is the reese84 POST's JSON reply.
type tokenReply struct {
	Token        string `json:"token"`
	CookieDomain string `json:"cookieDomain"`
}

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page := getChallengePage(hc)
	scriptURL := findScript(page)
	session := newSession(client, ip, page.URL)
	script := getScript(hc, page, scriptURL)
	solveChallenge(hc, session, page, scriptURL, script)
	reloadPage(hc, page)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS reese84 cookie set, page re-GET 200")
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

// findScript returns the script the challenge page adds with
// scriptElement.src = "...".
func findScript(page *httpclient.Response) *url.URL {
	match := challengeScript.FindStringSubmatch(page.Body)
	if match == nil {
		log.Fatalf("FAIL page returned %d without an Imperva challenge", page.Status)
	}
	return page.Resolve(match[1])
}

func getScript(hc *httpclient.Client, page *httpclient.Response, scriptURL *url.URL) string {
	res := hc.Get(scriptURL.String(), httpclient.ImpervaScript(acceptLanguage, page.URL))
	if res.Status != 200 {
		log.Fatalf("FAIL script returned %d", res.Status)
	}
	return res.Body
}

func solveChallenge(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, scriptURL *url.URL, script string) {
	res, err := session.Imperva(scriptURL.String(), script)
	if err != nil {
		log.Fatalf("FAIL imperva: %v", err)
	}
	// res.Payload is the POST body; it goes to the script's path with ?d=<host>.
	origin := scriptURL.Scheme + "://" + scriptURL.Host
	post := hc.Post(origin+scriptURL.Path+"?d="+scriptURL.Hostname(), httpclient.ImpervaPost(acceptLanguage, origin, page.URL), res.Payload)
	var reply tokenReply
	if err := json.Unmarshal([]byte(post.Body), &reply); err != nil || reply.Token == "" {
		log.Fatalf("FAIL reese84 POST returned %d: %s", post.Status, post.Body)
	}
	// The script stores the token as the reese84 cookie on cookieDomain.
	hc.SetCookie(reply.CookieDomain, "reese84", reply.Token)
}

func reloadPage(hc *httpclient.Client, page *httpclient.Response) {
	loaded := hc.Get(page.URL, httpclient.Navigate(acceptLanguage, ""))
	if loaded.Status != 200 {
		log.Fatalf("FAIL page re-GET returned %d", loaded.Status)
	}
	if challengeScript.MatchString(loaded.Body) {
		log.Fatal("FAIL page re-GET is still the challenge page")
	}
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
