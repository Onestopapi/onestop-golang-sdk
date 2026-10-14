// DataDome slider: the page answers 403 with `var dd={...rt:'c'}`. Load the
// captcha page, its script and both images, GET the captcha check with the
// generated query string, store the datadome cookie and load the page again.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Onestopapi/onestop-golang-sdk/examples/httpcloak/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "" // your identifier or account name
	pageURL        = "https://www.example.com/events/example"
	acceptLanguage = "en-GB,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

const geoOrigin = "https://geo.captcha-delivery.com"

var (
	ddObject      = regexp.MustCompile(`var\s+dd\s*=\s*(\{[^}]*\})`)
	challengePath = regexp.MustCompile(`captchaChallengePath: '([^']+)'`)
	captchaScript = regexp.MustCompile(`<script[^>]*\ssrc="([^"]*captcha[^"]*\.js)"`)
)

type challenge struct {
	RT     string `json:"rt"`
	CID    string `json:"cid"`
	Hash   string `json:"hsh"`
	T      string `json:"t"`
	S      int    `json:"s"`
	E      string `json:"e"`
	Cookie string `json:"cookie"`
}

// checkReply is the captcha check's JSON reply.
type checkReply struct {
	Cookie string `json:"cookie"`
}

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	session := newSession(client, ip, pageURL)
	page, dd := getBlockedPage(hc)
	geoURL, geo := getChallengePage(hc, dd)
	solveSlider(hc, session, page, geoURL, geo)
	status := reloadPage(hc)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS captcha check cookie, page re-GET", status)
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
	if dd.RT != "c" || dd.T == "bv" {
		log.Fatalf("FAIL challenge type %q t=%q is not the slider", dd.RT, dd.T)
	}
	return page, dd
}

func getChallengePage(hc *httpclient.Client, dd challenge) (string, *httpclient.Response) {
	geoURL := fmt.Sprintf("%s/captcha/?initialCid=%s&hash=%s&cid=%s&t=%s&referer=%s&s=%d",
		geoOrigin, url.QueryEscape(dd.CID), url.QueryEscape(dd.Hash), url.QueryEscape(dd.Cookie), dd.T, url.QueryEscape(pageURL), dd.S)
	if dd.E != "" {
		geoURL += "&e=" + url.QueryEscape(dd.E)
	}
	geoURL += "&dm=cd"

	u, _ := url.Parse(pageURL)
	geo := hc.Get(geoURL, httpclient.Frame(acceptLanguage, "cross-site", u.Scheme+"://"+u.Host+"/").StorageAccess())
	if geo.Status != 200 || !challengePath.MatchString(geo.Body) {
		log.Fatalf("FAIL captcha page returned %d without the slider challenge", geo.Status)
	}
	return geoURL, geo
}

func solveSlider(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, geoURL string, geo *httpclient.Response) {
	// Without a script tag the bundle is inline and the API reads it from HTML.
	var script string
	if match := captchaScript.FindStringSubmatch(geo.Body); match != nil {
		script = hc.Get(match[1], httpclient.Script(acceptLanguage, "same-site", geoOrigin+"/").StorageAccess()).Body
	}

	// The background is captchaChallengePath (.jpg); the puzzle piece is the
	// same path as .frag.png. Both go to the API as base64.
	path := challengePath.FindStringSubmatch(geo.Body)[1]
	var images []string
	for _, p := range []string{path, strings.Replace(path, ".jpg", ".frag.png", 1)} {
		image := hc.Get(geo.Resolve(p).String(), httpclient.Image(acceptLanguage, geoOrigin, "same-site", geoOrigin+"/"))
		if image.Status != 200 {
			log.Fatalf("FAIL captcha image returned %d", image.Status)
		}
		images = append(images, base64.StdEncoding.EncodeToString([]byte(image.Body)))
	}

	// Script and HTML are the captcha page's; PageResponse is the blocked page.
	res, err := session.DatadomeSlider(onestop.DatadomeSliderCall{
		Script:          script,
		HTML:            geo.Body,
		PageResponse:    page.Body,
		BackgroundImage: images[0],
		PuzzlePiece:     images[1],
	})
	if err != nil {
		log.Fatalf("FAIL slider: %v", err)
	}

	// res.Payload is the generated check query string.
	answer := hc.Get(geoOrigin+"/captcha/check"+res.Payload, httpclient.Post(acceptLanguage, "application/x-www-form-urlencoded; charset=UTF-8", "", geoURL).StorageAccess())
	var reply checkReply
	if err := json.Unmarshal([]byte(answer.Body), &reply); err != nil || answer.Status != 200 || reply.Cookie == "" {
		log.Fatalf("FAIL captcha check returned %d: %s", answer.Status, answer.Body)
	}
	// reply.Cookie is the datadome Set-Cookie line.
	cookie, err := http.ParseSetCookie(reply.Cookie)
	if err != nil {
		log.Fatalf("FAIL datadome cookie: %v", err)
	}
	hc.SetCookie(cookie.Domain, cookie.Name, cookie.Value)
}

func reloadPage(hc *httpclient.Client) int {
	again := hc.Get(pageURL, httpclient.Navigate(acceptLanguage, pageURL))
	if again.Status != 200 && again.Status != 302 {
		log.Fatalf("FAIL page re-GET returned %d", again.Status)
	}
	return again.Status
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
