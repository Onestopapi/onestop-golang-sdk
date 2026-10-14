// Cloudflare precursor: the page loads a /scripts/precursor/ script that
// makes up to five small POSTs. Each reply is {"i": seconds, "n": done}:
// n true stops; the API turns a long i into its sleep.
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
	identifier     = "" // your identifier or account name
	pageURL        = "https://www.example.com/"
	acceptLanguage = "en-US,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

// precursorScript finds the precursor script's path wherever the page writes
// it: a script tag or an inline loader that adds one, quoted either way.
var precursorScript = regexp.MustCompile(`(?:https://[A-Za-z0-9.-]+)?/cdn-cgi/challenge-platform/scripts/precursor/[A-Za-z0-9._/-]+`)

// precursorReply is a precursor POST's JSON reply: n is true when done.
type precursorReply struct {
	N bool `json:"n"`
}

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page, first := getChallengePage(hc)
	session := newSession(client, ip, page.URL)
	result := runChallengeLoop(hc, session, page, first)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS " + result)
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

// getChallengePage returns the page and the first call: the page and the
// precursor script.
func getChallengePage(hc *httpclient.Client) (*httpclient.Response, onestop.CloudflareCall) {
	page := hc.Get(pageURL, httpclient.Navigate(acceptLanguage, ""))
	path := precursorScript.FindString(page.Body)
	if page.Status != 200 || path == "" {
		log.Fatalf("FAIL page returned %d without a precursor script", page.Status)
	}
	script := hc.Get(page.Resolve(path).String(), httpclient.Script(acceptLanguage, "same-origin", page.URL))
	if script.Status != 200 {
		log.Fatalf("FAIL precursor script returned %d", script.Status)
	}
	return page, onestop.CloudflareCall{HTML: page.Body, Script: script.Body, ScriptURL: script.URL}
}

// runChallengeLoop sends each step's POSTs; every later call sends the
// previous context and the target's reply.
func runChallengeLoop(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, call onestop.CloudflareCall) string {
	u, _ := url.Parse(page.URL)
	origin := u.Scheme + "://" + u.Host
	for posts := 1; posts <= 5; posts++ {
		step, err := session.CloudflarePrecursor(call)
		if err != nil {
			log.Fatalf("FAIL precursor: %v", err)
		}
		call = onestop.CloudflareCall{Context: step.Context}
		for _, r := range step.Requests {
			res := hc.Do(r.Method, r.URL, httpclient.Post(acceptLanguage, "text/plain;charset=UTF-8", origin, page.URL).With(r.Headers), r.Body)
			var reply precursorReply
			if res.Status < 200 || res.Status > 299 || json.Unmarshal([]byte(res.Body), &reply) != nil {
				log.Fatalf("FAIL precursor POST %d returned %d: %s", posts, res.Status, res.Body)
			}
			if reply.N {
				return "precursor POSTs 2xx until n:true"
			}
			call.Response = json.RawMessage(res.Body)
		}
	}
	return "5 precursor POSTs 2xx"
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
