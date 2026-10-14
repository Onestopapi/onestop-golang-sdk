// SBSD block: POST one mode f payload to the block script with ?t=<token>
// (bm_sc 2~ clears the block), load the page again, then send the passive
// SBSD payloads (mode f, then t) for the real page's script.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/Onestopapi/onestop-golang-sdk/examples/tls-client/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "" // your identifier or account name
	pageURL        = "https://www.example.com/"
	acceptLanguage = "de-DE,de;q=0.9"

	proxy = "" // your proxy, in the site's country
)

var (
	sbsdScript = regexp.MustCompile(`<script[^>]*src="([^"]*\?v=[^"]*)"`)
	blockToken = regexp.MustCompile(`\{"cpr_chlge":"true","t":"([^"]*)"`)
)

// sbsdBody is the JSON body of an SBSD POST.
type sbsdBody struct {
	Body string `json:"body"`
}

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page := getBlockPage(hc)
	blockURL, token := findBlockScript(page)
	session := newSession(client, ip, page.URL)
	solveBlock(hc, session, page, blockURL, token)
	loaded, sbsdURL := reloadPage(hc, page)
	solvePassive(hc, session, loaded, sbsdURL)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS SBSD block cleared (bm_sc 2~), real page loaded, passive f+t 200")
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

func getBlockPage(hc *httpclient.Client) *httpclient.Response {
	return hc.Get(pageURL, httpclient.Navigate(acceptLanguage, ""))
}

func findBlockScript(page *httpclient.Response) (blockURL *url.URL, token string) {
	match := sbsdScript.FindStringSubmatch(page.Body)
	if match == nil {
		log.Fatal("FAIL not a block page")
	}
	blockURL = page.Resolve(match[1])
	// The block token is the script URL's t= value, else the page's
	// {"cpr_chlge":"true","t":"<token>"}.
	token = blockURL.Query().Get("t")
	if m := blockToken.FindStringSubmatch(page.Body); token == "" && m != nil {
		token = m[1]
	}
	if token == "" {
		log.Fatal("FAIL not a block page")
	}
	return blockURL, token
}

func solveBlock(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, blockURL *url.URL, token string) {
	origin := blockURL.Scheme + "://" + blockURL.Host
	block := hc.Get(blockURL.String(), httpclient.Script(acceptLanguage, "same-origin", page.URL))
	res, err := session.SbsdPayload(onestop.SbsdCall{
		UUID:       blockURL.Query().Get("v"),              // v= value of the script URL
		SbsdCookie: hc.Cookie(page.URL, "bm_so", "sbsd_o"), // cookie set by the page
		Script:     block.Body,
		Mode:       "f",
	})
	if err != nil {
		log.Fatalf("FAIL SBSD block: %v", err)
	}
	body, _ := json.Marshal(sbsdBody{res.Payload})
	post := hc.Post(origin+blockURL.Path+"?t="+token, httpclient.Post(acceptLanguage, "application/json", origin, page.URL), string(body))
	if post.Status != 200 {
		log.Fatalf("FAIL SBSD block POST returned %d", post.Status)
	}
	// bm_sc 2~ passed, 3~ failed.
	if verdict := hc.Cookie(page.URL, "bm_sc"); !strings.HasPrefix(verdict, "2~") {
		log.Fatalf("FAIL SBSD block verdict bm_sc=%s", verdict)
	}
}

func reloadPage(hc *httpclient.Client, page *httpclient.Response) (*httpclient.Response, *url.URL) {
	loaded := hc.Get(page.URL, httpclient.Navigate(acceptLanguage, ""))
	if loaded.Status != 200 {
		log.Fatalf("FAIL page re-GET returned %d", loaded.Status)
	}
	match := sbsdScript.FindStringSubmatch(loaded.Body)
	if match == nil {
		log.Fatal("FAIL real page has no SBSD script")
	}
	sbsdURL := loaded.Resolve(match[1])
	if sbsdURL.Query().Get("t") != "" {
		log.Fatal("FAIL page re-GET is still a block page")
	}
	return loaded, sbsdURL
}

func solvePassive(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, sbsdURL *url.URL) {
	origin := sbsdURL.Scheme + "://" + sbsdURL.Host
	sbsd := hc.Get(sbsdURL.String(), httpclient.Script(acceptLanguage, "same-origin", page.URL).NoCache())
	// uuid is the v= value of the new script URL; mode t sends f's context.
	call := onestop.SbsdCall{UUID: sbsdURL.Query().Get("v"), Script: sbsd.Body}
	for _, mode := range []string{"f", "t"} {
		call.Mode = mode
		call.SbsdCookie = hc.Cookie(page.URL, "bm_so", "sbsd_o")
		res, err := session.SbsdPayload(call)
		if err != nil {
			log.Fatalf("FAIL SBSD %s: %v", mode, err)
		}
		body, _ := json.Marshal(sbsdBody{res.Payload})
		post := hc.Post(origin+sbsdURL.Path, httpclient.Post(acceptLanguage, "application/json", origin, page.URL).NoCache(), string(body))
		if post.Status != 200 {
			log.Fatalf("FAIL SBSD %s POST returned %d", mode, post.Status)
		}
		call = onestop.SbsdCall{UUID: call.UUID, Context: res.Context}
	}
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
