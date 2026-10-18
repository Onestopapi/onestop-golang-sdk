// Cloudflare Turnstile: load the page, the widget's api.js and its iframe,
// then send the requests each step returns until the API returns the token.
package main

import (
	"bytes"
	"fmt"
	"image/png"
	"log"
	"math/rand/v2"
	"net/url"
	"strings"

	"github.com/Onestopapi/onestop-golang-sdk/examples/tls-client/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "YOUR_IDENTIFIER" // your identifier or account name
	pageURL        = "https://www.example.com/event"
	siteKey        = "YOUR_TURNSTILE_SITE_KEY" // from the page's turnstile.render options
	action         = "YOUR_TURNSTILE_ACTION"
	cData          = "YOUR_TURNSTILE_CDATA"
	acceptLanguage = "en-US,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page := getPage(hc)
	session := newSession(client, ip, page.URL)
	first := getChallengePage(hc, page)
	token := runChallengeLoop(hc, session, page, first)
	siteFirewall(hc, pageURL, token)
	fmt.Println("PASS Turnstile token returned")
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

// getChallengePage returns the first call: the widget iframe, its final URL
// and its cf-ray header.
func getChallengePage(hc *httpclient.Client, page *httpclient.Response) onestop.CloudflareCall {
	u, _ := url.Parse(page.URL)
	api := hc.Get("https://challenges.cloudflare.com/turnstile/v0/api.js", httpclient.Script(acceptLanguage, "cross-site", u.Scheme+"://"+u.Host+"/").StorageAccess())
	// api.js redirects to /turnstile/v0/<mode>/<version>/api.js.
	apiURL, _ := url.Parse(api.URL)
	segments := strings.Split(apiURL.Path, "/")
	if len(segments) != 6 {
		log.Fatalf("FAIL api.js resolved to %s", api.URL)
	}
	mode := segments[3]
	// rch is a random 5-character render id.
	rch := make([]byte, 5)
	for i := range rch {
		rch[i] = "abcdefghijklmnopqrstuvwxyz0123456789"[rand.IntN(36)]
	}
	frame := hc.Get("https://challenges.cloudflare.com/cdn-cgi/challenge-platform/h/"+mode+"/turnstile/f/av0/rch/"+string(rch)+"/"+siteKey+"/light/fbE/new/normal?lang=auto",
		httpclient.Frame(acceptLanguage, "cross-site", "").StorageAccess())
	if frame.Status != 200 {
		log.Fatalf("FAIL widget iframe returned %d", frame.Status)
	}
	return onestop.CloudflareCall{
		HTML:          frame.Body,
		ScriptURL:     api.URL,
		IframeURL:     frame.URL,
		BytecodeRayID: frame.Header.Get("cf-ray"),
		Action:        action,
		CData:         cData,
	}
}

// runChallengeLoop sends each step's requests; each reply goes back on the
// next call, by its purpose, until the API returns the token.
func runChallengeLoop(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, call onestop.CloudflareCall) string {
	frameURL := call.IframeURL
	step, err := session.CloudflareTurnstile(call)
	for err == nil && step.Token == "" {
		next := onestop.CloudflareCall{Context: step.Context, Action: action, CData: cData}
		for _, r := range step.Requests {
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
			}
		}
		step, err = session.CloudflareTurnstile(next)
	}
	if err != nil {
		log.Fatalf("FAIL Turnstile: %v", err)
	}
	return step.Token
}

func siteFirewall(hc *httpclient.Client, pageURL, token string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
