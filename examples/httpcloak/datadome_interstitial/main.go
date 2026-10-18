// DataDome interstitial: the page answers 403 with `var dd={...rt:'i'}`. Load
// the interstitial page and its script, POST the generated form body, store
// the returned datadome cookie and load the page again.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Onestopapi/onestop-golang-sdk/examples/httpcloak/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "YOUR_IDENTIFIER" // your identifier or account name
	pageURL        = "https://www.example.com/"
	acceptLanguage = "en-US,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

const geoOrigin = "https://geo.captcha-delivery.com"

var (
	ddObject = regexp.MustCompile(`var\s+dd\s*=\s*(\{[^}]*\})`)
	ddScript = regexp.MustCompile(`src="([^"]*interstitial\.[^"]*)"`)
)

type challenge struct {
	RT     string `json:"rt"`
	CID    string `json:"cid"`
	Hash   string `json:"hsh"`
	T      string `json:"t"`
	S      int    `json:"s"`
	E      string `json:"e"`
	B      int    `json:"b"`
	Cookie string `json:"cookie"`
}

// interstitialReply is the interstitial POST's JSON reply.
type interstitialReply struct {
	View   string `json:"view"`
	Cookie string `json:"cookie"`
}

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page, dd := getBlockedPage(hc)
	session := newSession(client, ip, page.URL)
	geoURL, geo := getChallengePage(hc, page, dd)
	solveInterstitial(hc, session, page, geoURL, geo)
	status := reloadPage(hc, page)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS interstitial view redirect, page re-GET", status)
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

func getBlockedPage(hc *httpclient.Client) (*httpclient.Response, challenge) {
	var dd challenge
	page := hc.Get(pageURL, httpclient.Navigate(acceptLanguage, ""))
	match := ddObject.FindStringSubmatch(page.Body)
	if page.Status != 403 || match == nil {
		log.Fatalf("FAIL page returned %d without a DataDome challenge", page.Status)
	}
	if err := json.Unmarshal([]byte(strings.ReplaceAll(match[1], "'", `"`)), &dd); err != nil {
		log.Fatalf("FAIL dd object: %v", err)
	}
	if dd.RT != "i" {
		log.Fatalf("FAIL challenge type %q is not the interstitial (t=%q)", dd.RT, dd.T)
	}
	return page, dd
}

func getChallengePage(hc *httpclient.Client, page *httpclient.Response, dd challenge) (string, *httpclient.Response) {
	geoURL := fmt.Sprintf("%s/interstitial/?initialCid=%s&hash=%s&cid=%s&referer=%s&s=%d",
		geoOrigin, url.QueryEscape(dd.CID), url.QueryEscape(dd.Hash), url.QueryEscape(dd.Cookie), url.QueryEscape(page.URL), dd.S)
	if dd.E != "" {
		geoURL += "&e=" + url.QueryEscape(dd.E)
	}
	geoURL += fmt.Sprintf("&b=%d&dm=cd", dd.B)

	// These pauses are browser pacing between the challenge requests, not the API's sleep.
	time.Sleep(400 * time.Millisecond)
	geo := hc.Get(geoURL, httpclient.Frame(acceptLanguage, "cross-site", page.URL).StorageAccess())
	if geo.Status != 200 {
		log.Fatalf("FAIL interstitial page returned %d", geo.Status)
	}
	return geoURL, geo
}

func solveInterstitial(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, geoURL string, geo *httpclient.Response) {
	// Without a script tag the bundle is inline and the API reads it from HTML.
	var script string
	if match := ddScript.FindStringSubmatch(geo.Body); match != nil {
		time.Sleep(200 * time.Millisecond)
		script = hc.Get(match[1], httpclient.Script(acceptLanguage, "same-site", geoOrigin+"/").StorageAccess()).Body
	}

	time.Sleep(400 * time.Millisecond)
	// Script and HTML are the interstitial's; PageResponse is the blocked page.
	res, err := session.DatadomeInterstitial(onestop.DatadomeInterstitialCall{
		Script:       script,
		HTML:         geo.Body,
		PageResponse: page.Body,
	})
	if err != nil {
		log.Fatalf("FAIL interstitial: %v", err)
	}

	time.Sleep(time.Second)
	// res.Payload is the generated form body.
	post := hc.Post(geoOrigin+"/interstitial/", httpclient.Post(acceptLanguage, "application/x-www-form-urlencoded; charset=UTF-8", geoOrigin, geoURL).StorageAccess(), res.Payload)
	var reply interstitialReply
	if err := json.Unmarshal([]byte(post.Body), &reply); err != nil || reply.View != "redirect" {
		log.Fatalf("FAIL interstitial POST returned %d, view %q: %s", post.Status, reply.View, post.Body)
	}
	// reply.Cookie is the datadome Set-Cookie line.
	cookie, err := http.ParseSetCookie(reply.Cookie)
	if err != nil {
		log.Fatalf("FAIL datadome cookie: %v", err)
	}
	hc.SetCookie(cookie.Domain, cookie.Name, cookie.Value)
}

func reloadPage(hc *httpclient.Client, page *httpclient.Response) int {
	again := hc.Get(page.URL, httpclient.Navigate(acceptLanguage, page.URL))
	if again.Status != 200 && again.Status != 302 {
		log.Fatalf("FAIL page re-GET returned %d", again.Status)
	}
	return again.Status
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
