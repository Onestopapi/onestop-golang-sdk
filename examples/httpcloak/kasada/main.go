// Kasada: load the page and its p.js, then mfc, fp (429) and ips.js, and
// POST the generated /tl body. A page that is already the 429 block page
// links ips.js itself. /tl answers 200 with reload:true. Then build the
// x-kpsdk-cd header locally and make one protected POST.
package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Onestopapi/onestop-golang-sdk/examples/httpcloak/httpclient"
	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

const (
	apiKey         = "YOUR_API_KEY"
	identifier     = "YOUR_IDENTIFIER" // your identifier or account name
	pageURL        = "https://www.example.com/"
	protectedURL   = "https://www.example.com/api/protected" // a request Kasada protects
	protectedBody  = "{}"                                    // its JSON body
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
	page := getPage(hc)
	session := newSession(client, ip, page.URL)
	base := findBase(page)
	ps := getPScript(hc, session, page, base)
	script, h := getIps(hc, page, base, ps)
	tl := postTl(hc, session, page, script)
	cd := buildCDHeader(ps, tl)
	postProtected(hc, page, ps, tl, h, cd)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS /tl 200 reload:true, protected request 200")
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

func isBlockPage(page *httpclient.Response) bool {
	return page.Status == 429 && ipsScript.MatchString(page.Body)
}

// findBase returns the script directory: p.js, mfc, fp, ips.js and tl sit in
// it. A block page links ips.js there; any other page links p.js.
func findBase(page *httpclient.Response) string {
	if isBlockPage(page) {
		base, _, _ := strings.Cut(ipsURLFrom(page), "ips.js")
		return base
	}
	// p.js lives at /<uuid>/<uuid>/p.js.
	// Some pages write the path JSON-escaped (`\/` or `\u002f` for `/`).
	path := pjsPath.FindString(strings.NewReplacer(`\/`, "/", `\u002f`, "/").Replace(page.Body))
	if path == "" {
		log.Fatalf("FAIL page returned %d without a Kasada p.js", page.Status)
	}
	u, _ := url.Parse(page.URL)
	return u.Scheme + "://" + u.Host + strings.TrimSuffix(path, "p.js")
}

// getPScript loads p.js. Keep the result: x-kpsdk-v goes on the Kasada
// requests and the CD header is built from it.
func getPScript(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, base string) *onestop.KasadaPScriptResponse {
	pjs := hc.Get(base+"p.js", httpclient.Script(acceptLanguage, "same-origin", page.URL))
	ps, err := session.KasadaPScript(pjs.Body)
	if err != nil {
		log.Fatalf("FAIL p.js: %v", err)
	}
	return ps
}

// getIps loads ips.js and returns it with the x-kpsdk-h from mfc. A block
// page links it directly (no mfc); otherwise mfc and fp lead to it.
func getIps(hc *httpclient.Client, page *httpclient.Response, base string, ps *onestop.KasadaPScriptResponse) (ips, string) {
	from, h := page, ""
	if !isBlockPage(page) {
		from, h = getFp(hc, page, base, ps.Headers["x-kpsdk-v"])
	}
	ipsURL := ipsURLFrom(from)
	res := hc.Get(ipsURL, httpclient.Script(acceptLanguage, "same-origin", from.URL))
	if res.Status != 200 {
		log.Fatalf("FAIL ips.js returned %d", res.Status)
	}
	return ips{ipsURL, res.Body, from.URL}, h
}

// getFp loads mfc and fp and returns fp with mfc's x-kpsdk-h, which the
// protected requests carry. fp answers 429 with a body that links ips.js.
func getFp(hc *httpclient.Client, page *httpclient.Response, base, version string) (*httpclient.Response, string) {
	mfc := hc.Get(base+"mfc", httpclient.KasadaMfc(acceptLanguage, version, page.URL))
	if mfc.Status != 200 {
		log.Fatalf("FAIL mfc returned %d", mfc.Status)
	}
	fp := hc.Get(base+"fp?x-kpsdk-v="+version, httpclient.Frame(acceptLanguage, "same-origin", page.URL))
	if !ipsScript.MatchString(fp.Body) {
		log.Fatal("FAIL fp returned no ips.js")
	}
	return fp, mfc.Header.Get("x-kpsdk-h")
}

// ipsURLFrom returns the ips.js src a Kasada 429 body links, HTML-unescaped
// and resolved against the response URL.
func ipsURLFrom(res *httpclient.Response) string {
	return res.Resolve(ipsScript.FindStringSubmatch(res.Body)[1]).String()
}

func postTl(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, script ips) *httpclient.Response {
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
	return res
}

// buildCDHeader builds x-kpsdk-cd locally (no API call) from the p.js result
// and the /tl response's x-kpsdk-ct and x-kpsdk-st.
func buildCDHeader(ps *onestop.KasadaPScriptResponse, tl *httpclient.Response) string {
	st, err := strconv.ParseInt(tl.Header.Get("x-kpsdk-st"), 10, 64) // ms
	if err != nil {
		log.Fatalf("FAIL /tl x-kpsdk-st: %v", err)
	}
	cd, err := ps.CDHeader(tl.Header.Get("x-kpsdk-ct"), st)
	if err != nil {
		log.Fatalf("FAIL cd header: %v", err)
	}
	return cd
}

// postProtected is the request Kasada protects, a POST as most are:
// x-kpsdk-ct (the newest one seen, here from /tl), x-kpsdk-v from p.js,
// x-kpsdk-h from mfc and the CD header just built. Each protected POST gets
// a fresh CD header.
func postProtected(hc *httpclient.Client, page *httpclient.Response, ps *onestop.KasadaPScriptResponse, tl *httpclient.Response, h, cd string) {
	u, _ := url.Parse(page.URL)
	headers := httpclient.Post(acceptLanguage, "application/json", u.Scheme+"://"+u.Host, page.URL).With(map[string]string{
		"x-kpsdk-ct": tl.Header.Get("x-kpsdk-ct"),
		"x-kpsdk-cd": cd,
		"x-kpsdk-h":  h,
		"x-kpsdk-v":  ps.Headers["x-kpsdk-v"],
	})
	res := hc.Post(protectedURL, headers, protectedBody)
	if res.Status != 200 {
		log.Fatalf("FAIL protected request returned %d", res.Status)
	}
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
}
