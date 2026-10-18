// Akamai: load the homepage, send the SBSD payloads (mode f, then t), then
// POST sensor data until the sensor is accepted.
package main

import (
	"encoding/json"
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
	identifier     = "YOUR_IDENTIFIER" // your identifier or account name
	pageURL        = "https://www.example.com/"
	acceptLanguage = "en-US,en;q=0.9"

	proxy = "" // your proxy, in the site's country
)

var (
	sensorScript = regexp.MustCompile(`<script type="text/javascript"[^>]*src="([A-Za-z0-9/_-]+)"></script>`)
	sbsdScript   = regexp.MustCompile(`<script[^>]*src="([^"]*\?v=[^"]*)"`)
)

// sbsdBody is the JSON body of an SBSD POST.
type sbsdBody struct {
	Body string `json:"body"`
}

// sensorBody is the JSON body of a sensor POST.
type sensorBody struct {
	SensorData string `json:"sensor_data"`
}

func main() {
	hc := httpclient.New(proxy)
	client, ip := newClient()
	page := getHomePage(hc)
	sensorURL, sbsdURL := findScripts(page)
	session := newSession(client, ip, page.URL)
	sensor, sbsd := getScripts(hc, page, sensorURL, sbsdURL)
	solveSbsdPassive(hc, session, page, sbsdURL, sbsd)
	postSensors(hc, session, page, sensorURL, sensor)
	siteFirewall(hc, pageURL)
	fmt.Println("PASS sensor accepted")
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

func getHomePage(hc *httpclient.Client) *httpclient.Response {
	return hc.Get(pageURL, httpclient.Navigate(acceptLanguage, "").NoCache())
}

func findScripts(page *httpclient.Response) (sensorURL, sbsdURL *url.URL) {
	sensorMatch := sensorScript.FindStringSubmatch(page.Body)
	sbsdMatch := sbsdScript.FindStringSubmatch(page.Body)
	if page.Status != 200 || sensorMatch == nil || sbsdMatch == nil {
		log.Fatalf("FAIL homepage returned %d without the sensor and SBSD scripts", page.Status)
	}
	return page.Resolve(sensorMatch[1]), page.Resolve(sbsdMatch[1])
}

func getScripts(hc *httpclient.Client, page *httpclient.Response, sensorURL, sbsdURL *url.URL) (sensor, sbsd string) {
	sensor = hc.Get(sensorURL.String(), httpclient.Script(acceptLanguage, "same-origin", page.URL).NoCache()).Body
	sbsd = hc.Get(sbsdURL.String(), httpclient.Script(acceptLanguage, "same-origin", page.URL).NoCache()).Body
	return sensor, sbsd
}

func solveSbsdPassive(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, sbsdURL *url.URL, script string) {
	u, _ := url.Parse(page.URL)
	origin := u.Scheme + "://" + u.Host
	// uuid is the v= value of the SBSD script URL; mode t sends f's context.
	call := onestop.SbsdCall{UUID: sbsdURL.Query().Get("v"), Script: script}
	for _, mode := range []string{"f", "t"} {
		call.Mode = mode
		call.SbsdCookie = hc.Cookie(page.URL, "bm_so", "sbsd_o") // cookie set by the page
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

func postSensors(hc *httpclient.Client, session onestop.Session, page *httpclient.Response, sensorURL *url.URL, script string) {
	origin := sensorURL.Scheme + "://" + sensorURL.Host
	// The first sensor call sends the script, every later call the previous
	// context instead. _abck and bm_sz are cookies set by the site.
	call := onestop.AkamaiSensorCall{Script: script, ScriptURL: sensorURL.String()}
	for n := 1; ; n++ {
		call.Abck = hc.Cookie(page.URL, "_abck")
		call.Bmsz = hc.Cookie(page.URL, "bm_sz")
		res, err := session.AkamaiSensor(call)
		if err != nil {
			log.Fatalf("FAIL sensor: %v", err)
		}
		body, _ := json.Marshal(sensorBody{res.Payload})
		post := hc.Post(sensorURL.String(), httpclient.Post(acceptLanguage, "text/plain;charset=UTF-8", origin, page.URL).NoCache(), string(body))
		if post.Status != 200 && post.Status != 201 {
			log.Fatalf("FAIL sensor POST returned %d", post.Status)
		}
		// Accepted: _abck contains ~0~ or the reply is exactly {"success": true}.
		if strings.Contains(hc.Cookie(page.URL, "_abck"), "~0~") || post.Body == `{"success": true}` {
			return
		}
		if n == 8 {
			log.Fatal("FAIL sensor not accepted after 8 POSTs")
		}
		call = onestop.AkamaiSensorCall{ScriptURL: sensorURL.String(), Context: res.Context}
	}
}

func siteFirewall(hc *httpclient.Client, pageURL string) {
	// Insert the relevant requests for your site's firewall. This is up to you and your agent to experiment with.
	// Some sites (DataDome, Akamai) are strict on flow: you might need additional requests here.
	// If a later request resets _abck, run one more sensor call with the last context plus one POST.
}
