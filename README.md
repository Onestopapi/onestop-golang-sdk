# OneStop Go SDK

Go client for the [OneStop Antibot](https://onestopantibot.com) API. The SDK calls `/generate` and returns typed results; your application fetches the target pages and scripts and sends the generated payloads.

Products: Akamai (sensor, SBSD, Pixel, sec-cpt), DataDome (tags, interstitial, slider), Cloudflare (WAF, Turnstile, JSD, precursor), Kasada (p.js, /tl, CD header), Imperva (reese84, utmvc).

## Install

```sh
go get github.com/Onestopapi/onestop-golang-sdk/onestop
```

Requires Go 1.25+. Reference: [pkg.go.dev](https://pkg.go.dev/github.com/Onestopapi/onestop-golang-sdk/onestop).

## Quick start

```go
client, err := onestop.New(onestop.Config{
	APIKey:     "YOUR_API_KEY",
	Identifier: "YOUR_IDENTIFIER", // your identifier or account name
})
if err != nil {
	log.Fatal(err)
}
ip, err := client.GetIP() // set Config.IPHTTPClient to your proxy so this is the proxy's IP
if err != nil {
	log.Fatal(err)
}
session := client.Session(onestop.Session{
	Site: pageURL, Referer: pageURL, UserAgent: userAgent, AcceptLanguage: "en-US,en;q=0.9", IP: ip,
})
res, err := session.AkamaiSensor(onestop.AkamaiSensorCall{Abck: abck, Bmsz: bmsz, Script: script, ScriptURL: scriptURL})
// POST res.Payload as sensor_data; send res.Context on the next call.
```

## Header order

Copy each request's header names, values and order from a browser recording (DevTools, powhttp, Charles) of that request, and set the order explicitly in your TLS client; the examples' headers only show how:

```go
// tls-client
req.Header = fhttp.Header{
	"user-agent":         {userAgent},
	"accept":             {"*/*"},
	fhttp.HeaderOrderKey: {"user-agent", "accept"},
}

// httpcloak
req := &httpcloak.Request{Method: "GET", URL: pageURL,
	Headers:     map[string][]string{"user-agent": {userAgent}, "accept": {"*/*"}},
	HeaderOrder: []string{"user-agent", "accept"},
}
```

See [examples/](examples) for full flows per product and TLS client.

Documentation: [onestopantibot.com/dashboard/docs](https://onestopantibot.com/dashboard/docs)
