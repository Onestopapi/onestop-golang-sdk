// Cloudflare WAF: the page answers 403 with the managed challenge. Load the
// challenge script, then send the requests each step returns until the form
// POST that sets cf_clearance.
package main

import (
	"bytes"
	"fmt"
	"image/png"
	"log"
	"regexp"

	"github.com/Onestopapi/onestop-golang-sdk/examples/httpcloak/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "YOUR_IDENTIFIER" // your identifier or account name
	pageURL        = "https://www.example.com/login"
	acceptLanguage = "en-US,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

var (
	challengeScript = regexp.MustCompile(`src = '(/cdn-cgi/challenge-platform/[^']+)'`)
	challengeToken  = regexp.MustCompile(`__cf_chl_rt_tk=([^"&]+)`)
)

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page, first := getChallengePage(hc)
	session := newSession(client, ip, page.URL)
	runChallengeLoop(hc, session, page, first)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS cf_clearance set")
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

// getChallengePage returns the page and the first call: the challenge page
// and its script.
func getChallengePage(hc *httpclient.Client) (*httpclient.Response, onestop.CloudflareCall) {
	page := hc.Get(pageURL, httpclient.Navigate(acceptLanguage, ""))
	// The challenge page has the script path (src = '...') and the
	// __cf_chl_rt_tk token.
	scriptMatch := challengeScript.FindStringSubmatch(page.Body)
	tokenMatch := challengeToken.FindStringSubmatch(page.Body)
	if scriptMatch == nil || tokenMatch == nil {
		log.Fatalf("FAIL page returned %d without a Cloudflare challenge", page.Status)
	}
	scriptURL := page.Resolve(scriptMatch[1]).String()
	script := hc.Get(scriptURL, httpclient.ChallengeScript(acceptLanguage, page.URL+"?__cf_chl_rt_tk="+tokenMatch[1]))
	return page, onestop.CloudflareCall{HTML: page.Body, Script: script.Body, ScriptURL: scriptURL}
}

// runChallengeLoop sends each step's requests; each reply goes back on the
// next call, by its purpose. The form POST sets cf_clearance.
func runChallengeLoop(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, call onestop.CloudflareCall) {
	step, err := session.CloudflareWaf(call)
	frameURL := ""
	for err == nil && step.Token == "" {
		next := onestop.CloudflareCall{Context: step.Context}
		for _, r := range step.Requests {
			if r.Purpose == "form" {
				hc.FollowRedirects(false)
			}
			res := hc.Do(r.Method, r.URL, httpclient.Cloudflare(r, acceptLanguage, page.URL, frameURL), r.Body)
			switch r.Purpose {
			case "challenge":
				if res.Status != 200 {
					log.Fatalf("FAIL challenge request returned %d", res.Status)
				}
				next.Response = res.Body
			case "iframe":
				frameURL = res.URL
				next.HTML = res.Body
				next.IframeURL = res.URL
				next.BytecodeRayID = res.Header.Get("cf-ray")
			case "image":
				size, err := png.DecodeConfig(bytes.NewReader([]byte(res.Body)))
				if err != nil {
					log.Fatalf("FAIL challenge image: %v", err)
				}
				next.ImageWidth, next.ImageHeight = onestop.Ptr(size.Width), onestop.Ptr(size.Height)
			case "pat":
				next.PatStatus = onestop.Ptr(res.Status) // a 4xx is normal
			case "form":
				if hc.Cookie(page.URL, "cf_clearance") == "" {
					log.Fatalf("FAIL form POST returned %d without cf_clearance", res.Status)
				}
				return
			}
		}
		step, err = session.CloudflareWaf(next)
	}
	if err != nil {
		log.Fatalf("FAIL WAF: %v", err)
	}
	log.Fatal("FAIL the challenge ended without a form POST")
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
