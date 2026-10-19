// Header sets and order here come from one Chrome recording; copy yours from a browser recording of each request you make.

// Package httpclient sends the examples' target requests with
// github.com/sardanioss/httpcloak (Chrome TLS and HTTP/2 fingerprint, one
// cookie jar) and holds Chrome's headers for each kind of request.
package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sardanioss/httpcloak"

	"github.com/Onestopapi/onestop-golang-sdk/onestop"
)

type Response struct {
	Status int
	Header http.Header
	URL    string // final URL, after redirects
	Body   string
}

// Resolve returns a URL found in the body (HTML-escaped, maybe relative) as
// an absolute URL.
func (r *Response) Resolve(ref string) *url.URL {
	base, _ := url.Parse(r.URL)
	u, err := base.Parse(html.UnescapeString(ref))
	if err != nil {
		log.Fatal(err)
	}
	return u
}

type Client struct {
	session *httpcloak.Session
}

// New returns a client that follows redirects. proxy is "" or a proxy URL.
// TLS-only mode: httpcloak adds no headers of its own; HTTP/3 is off so
// every request uses the proxy's TCP tunnel.
func New(proxy string) *Client {
	options := []httpcloak.SessionOption{
		httpcloak.WithTLSOnly(),
		httpcloak.WithDisableHTTP3(),
		httpcloak.WithoutRetry(),
		httpcloak.WithoutConditionalCache(),
		httpcloak.WithSessionTimeout(30 * time.Second),
	}
	if proxy != "" {
		options = append(options, httpcloak.WithSessionProxy(proxy))
	}
	return &Client{httpcloak.NewSession("chrome-155-windows", options...)}
}

// IPClient is the net/http client for onestop.Client.GetIP, through the
// same proxy, so the API sees the IP the target sees.
func IPClient(proxy string) *http.Client {
	if proxy == "" {
		return &http.Client{}
	}
	u, err := url.Parse(proxy)
	if err != nil {
		log.Fatal(err)
	}
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
}

func (c *Client) Get(rawURL string, headers Headers) *Response {
	return c.Do("GET", rawURL, headers, "")
}

func (c *Client) Post(rawURL string, headers Headers, body string) *Response {
	return c.Do("POST", rawURL, headers, body)
}

// Do sends one request with the headers in the given order, content-length
// first and cookie right after accept-language. Headers with an empty value
// are left out. A transport error ends the program.
func (c *Client) Do(method, rawURL string, headers Headers, body string) *Response {
	req := &httpcloak.Request{Method: method, URL: rawURL, Headers: map[string][]string{}, HeaderOrder: []string{"content-length"}}
	if method != "GET" {
		req.Body = strings.NewReader(body)
	}
	for _, h := range headers {
		if h[1] == "" {
			continue
		}
		req.Headers[h[0]] = []string{h[1]}
		req.HeaderOrder = append(req.HeaderOrder, h[0])
		if h[0] == "accept-language" {
			req.HeaderOrder = append(req.HeaderOrder, "cookie")
		}
	}
	res, err := c.session.Do(context.Background(), req)
	if err != nil {
		log.Fatalf("FAIL %s %s: %v", method, rawURL, err)
	}
	data, err := res.Text()
	if err != nil {
		log.Fatalf("FAIL %s %s: %v", method, rawURL, err)
	}
	header := http.Header{}
	for name, values := range res.Headers {
		for _, v := range values {
			header.Add(name, v)
		}
	}
	fmt.Println(method, rawURL, res.StatusCode)
	return &Response{Status: res.StatusCode, Header: header, URL: res.FinalURL, Body: data}
}

// FollowRedirects turns redirect following on or off for later requests.
func (c *Client) FollowRedirects(follow bool) {
	c.session.SetFollowRedirects(follow)
}

// Cookie returns the first of the named cookies stored for rawURL's host,
// or "".
func (c *Client) Cookie(rawURL string, names ...string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Fatal(err)
	}
	cookies := c.session.GetCookies()
	for _, name := range names {
		for _, cookie := range cookies {
			domain := strings.TrimPrefix(cookie.Domain, ".")
			if cookie.Name == name && (u.Hostname() == domain || strings.HasSuffix(u.Hostname(), "."+domain)) {
				return cookie.Value
			}
		}
	}
	return ""
}

// Cookies returns the cookies stored for rawURL whose name starts with prefix,
// in browser order: longer path first, then oldest first. GetCookies sorts by
// name, so the order comes from the session export, which keeps each cookie's
// creation time and creation order.
func (c *Client) Cookies(rawURL, prefix string) []onestop.ImpervaSessionCookie {
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Fatal(err)
	}
	data, err := c.session.Marshal()
	if err != nil {
		log.Fatal(err)
	}
	var export struct {
		Cookies map[string][]exportedCookie `json:"cookies"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		log.Fatal(err)
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	var matched []exportedCookie
	for _, cookies := range export.Cookies {
		for _, cookie := range cookies {
			domain := strings.TrimPrefix(cookie.Domain, ".")
			if strings.HasPrefix(cookie.Name, prefix) && (u.Hostname() == domain || strings.HasSuffix(u.Hostname(), "."+domain)) && strings.HasPrefix(path, cookie.Path) {
				matched = append(matched, cookie)
			}
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if len(matched[i].Path) != len(matched[j].Path) {
			return len(matched[i].Path) > len(matched[j].Path)
		}
		return matched[i].CreatedAt.Before(matched[j].CreatedAt)
	})
	out := make([]onestop.ImpervaSessionCookie, len(matched))
	for i, cookie := range matched {
		out[i] = onestop.ImpervaSessionCookie{Name: cookie.Name, Value: cookie.Value}
	}
	return out
}

// exportedCookie is one cookie of Session.Marshal's "cookies", which lists each
// host's cookies in creation order.
type exportedCookie struct {
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	Domain    string    `json:"domain"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

// SetCookie stores a cookie for domain and its subdomains.
func (c *Client) SetCookie(domain, name, value string) {
	c.session.SetCookie(httpcloak.CookieInfo{Name: name, Value: value, Domain: domain, Path: "/"})
}

// Chrome 155 on Windows.
const (
	UserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0.0.0 Safari/537.36"
	secChUa        = `"Google Chrome";v="155", "Chromium";v="155", "Not(A:Brand";v="24"`
	secChUaFull    = `"Google Chrome";v="155.0.8059.39", "Chromium";v="155.0.8059.39", "Not(A:Brand";v="24.0.0.0"`
	secChUaVersion = `"155.0.8059.39"`
	platform       = `"Windows"`
	acceptDocument = "text/html,application/xhtml+xml,application/xml;q=0.9,image/jxl,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	acceptImage    = "image/jxl,image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	acceptEncoding = "gzip, deflate, br, zstd"
)

// Headers are sent in this order.
type Headers [][2]string

// Navigate is a top-level page load: referer "" for a typed URL, else a
// same-origin link.
func Navigate(acceptLanguage, referer string) Headers {
	site := "same-origin"
	if referer == "" {
		site = "none"
	}
	return Headers{
		{"sec-ch-ua", secChUa},
		{"sec-ch-ua-mobile", "?0"},
		{"sec-ch-ua-platform", platform},
		{"upgrade-insecure-requests", "1"},
		{"user-agent", UserAgent},
		{"accept", acceptDocument},
		{"sec-fetch-site", site},
		{"sec-fetch-mode", "navigate"},
		{"sec-fetch-user", "?1"},
		{"sec-fetch-dest", "document"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "u=0, i"},
	}
}

// Frame is an iframe document load.
func Frame(acceptLanguage, site, referer string) Headers {
	return Headers{
		{"sec-ch-ua", secChUa},
		{"sec-ch-ua-mobile", "?0"},
		{"sec-ch-ua-platform", platform},
		{"upgrade-insecure-requests", "1"},
		{"user-agent", UserAgent},
		{"accept", acceptDocument},
		{"sec-fetch-site", site},
		{"sec-fetch-mode", "navigate"},
		{"sec-fetch-dest", "iframe"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "u=0, i"},
	}
}

// Script is a <script src> load.
func Script(acceptLanguage, site, referer string) Headers {
	return Headers{
		{"sec-ch-ua-platform", platform},
		{"user-agent", UserAgent},
		{"sec-ch-ua", secChUa},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
		{"sec-fetch-site", site},
		{"sec-fetch-mode", "no-cors"},
		{"sec-fetch-dest", "script"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "u=1"},
	}
}

// Post is a same-origin fetch from the page; a GET fetch has no origin.
func Post(acceptLanguage, contentType, origin, referer string) Headers {
	return Headers{
		{"sec-ch-ua-platform", platform},
		{"user-agent", UserAgent},
		{"sec-ch-ua", secChUa},
		{"content-type", contentType},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
		{"origin", origin},
		{"sec-fetch-site", "same-origin"},
		{"sec-fetch-mode", "cors"},
		{"sec-fetch-dest", "empty"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "u=1, i"},
	}
}

// Image is an <img> load; with an origin it is a CORS image.
func Image(acceptLanguage, origin, site, referer string) Headers {
	mode := "no-cors"
	if origin != "" {
		mode = "cors"
	}
	return Headers{
		{"origin", origin},
		{"sec-ch-ua-platform", platform},
		{"user-agent", UserAgent},
		{"sec-ch-ua", secChUa},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", acceptImage},
		{"sec-fetch-site", site},
		{"sec-fetch-mode", mode},
		{"sec-fetch-dest", "image"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "i"},
	}
}

// KasadaMfc is the mfc fetch; version is x-kpsdk-v from the p.js result.
func KasadaMfc(acceptLanguage, version, referer string) Headers {
	return Headers{
		{"x-kpsdk-v", version},
		{"sec-ch-ua-platform", platform},
		{"user-agent", UserAgent},
		{"x-kpsdk-h", "01"},
		{"sec-ch-ua", secChUa},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
		{"sec-fetch-site", "same-origin"},
		{"sec-fetch-mode", "cors"},
		{"sec-fetch-dest", "empty"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "u=1, i"},
	}
}

// KasadaTL is the /tl POST with the x-kpsdk-* headers from the API.
func KasadaTL(acceptLanguage string, kasada map[string]string, origin, referer string) Headers {
	return Headers{
		{"x-kpsdk-ct", kasada["x-kpsdk-ct"]},
		{"sec-ch-ua-platform", platform},
		{"x-kpsdk-dt", kasada["x-kpsdk-dt"]},
		{"sec-ch-ua", secChUa},
		{"x-kpsdk-im", kasada["x-kpsdk-im"]},
		{"sec-ch-ua-mobile", "?0"},
		{"x-kpsdk-v", kasada["x-kpsdk-v"]},
		{"user-agent", UserAgent},
		{"content-type", "application/octet-stream"},
		{"accept", "*/*"},
		{"origin", origin},
		{"sec-fetch-site", "same-origin"},
		{"sec-fetch-mode", "cors"},
		{"sec-fetch-dest", "empty"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "u=1, i"},
	}.With(kasada)
}

// ImpervaScript is the reese84 script, which the challenge page adds as an
// async script; Chrome sends no priority header for it.
func ImpervaScript(acceptLanguage, referer string) Headers {
	return Headers{
		{"sec-ch-ua-platform", platform},
		{"user-agent", UserAgent},
		{"sec-ch-ua", secChUa},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
		{"sec-fetch-site", "same-origin"},
		{"sec-fetch-mode", "no-cors"},
		{"sec-fetch-dest", "script"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
	}
}

// ImpervaPost is the reese84 POST; the script sets accept and content-type.
func ImpervaPost(acceptLanguage, origin, referer string) Headers {
	return Headers{
		{"sec-ch-ua-platform", platform},
		{"user-agent", UserAgent},
		{"accept", "application/json; charset=utf-8"},
		{"sec-ch-ua", secChUa},
		{"content-type", "text/plain; charset=utf-8"},
		{"sec-ch-ua-mobile", "?0"},
		{"origin", origin},
		{"sec-fetch-site", "same-origin"},
		{"sec-fetch-mode", "cors"},
		{"sec-fetch-dest", "empty"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
		{"priority", "u=1, i"},
	}
}

// ChallengeScript is the Cloudflare challenge script. The challenge page
// asks for every client hint (Accept-CH), so the full set is sent.
func ChallengeScript(acceptLanguage, referer string) Headers {
	return Headers{
		{"sec-ch-ua-full-version-list", secChUaFull},
		{"sec-ch-ua-platform", platform},
		{"sec-ch-ua", secChUa},
		{"sec-ch-ua-bitness", `"64"`},
		{"sec-ch-ua-model", `""`},
		{"sec-ch-ua-mobile", "?0"},
		{"sec-ch-ua-arch", `"x86"`},
		{"sec-ch-ua-full-version", secChUaVersion},
		{"user-agent", UserAgent},
		{"sec-ch-ua-platform-version", `"19.0.0"`},
		{"accept", "*/*"},
		{"sec-fetch-site", "same-origin"},
		{"sec-fetch-mode", "no-cors"},
		{"sec-fetch-dest", "script"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", acceptLanguage},
	}
}

// Cloudflare is one request of a Cloudflare step, by its Purpose, with the
// request's own headers (cf-chl, the form's origin and referer, and the like)
// set on top. frameURL is the challenge iframe's URL.
func Cloudflare(r onestop.CloudflareHTTPRequest, acceptLanguage, pageURL, frameURL string) Headers {
	page, _ := url.Parse(pageURL)
	target, _ := url.Parse(r.URL)
	var h Headers
	switch {
	case r.Purpose == "form":
		h = Headers{
			{"sec-ch-ua", secChUa},
			{"sec-ch-ua-mobile", "?0"},
			{"sec-ch-ua-full-version", secChUaVersion},
			{"sec-ch-ua-arch", `"x86"`},
			{"sec-ch-ua-full-version-list", secChUaFull},
			{"sec-ch-ua-platform", platform},
			{"sec-ch-ua-platform-version", `"19.0.0"`},
			{"sec-ch-ua-bitness", `"64"`},
			{"sec-ch-ua-model", `""`},
			{"origin", ""},
			{"content-type", "application/x-www-form-urlencoded"},
			{"upgrade-insecure-requests", "1"},
			{"user-agent", UserAgent},
			{"referer", ""},
		}
	case r.Purpose == "iframe":
		h = Frame(acceptLanguage, "cross-site", "").StorageAccess()
	case r.Purpose == "image":
		h = Image(acceptLanguage, "", "same-origin", frameURL).StorageAccess()
	case r.Purpose == "pat":
		h = Headers{
			{"cache-control", "max-age=0"},
			{"sec-ch-ua-platform", platform},
			{"user-agent", UserAgent},
			{"sec-ch-ua", secChUa},
			{"sec-ch-ua-mobile", "?0"},
			{"accept", "*/*"},
			{"sec-fetch-site", "same-origin"},
			{"sec-fetch-mode", "cors"},
			{"sec-fetch-dest", "empty"},
			{"sec-fetch-storage-access", "active"},
			{"referer", frameURL},
			{"accept-encoding", acceptEncoding},
			{"accept-language", acceptLanguage},
			{"priority", "u=1, i"},
		}
	case target.Host == page.Host: // challenge POST from the page
		h = Headers{
			{"sec-ch-ua-full-version-list", secChUaFull},
			{"sec-ch-ua-platform", platform},
			{"sec-ch-ua", secChUa},
			{"sec-ch-ua-bitness", `"64"`},
			{"cf-chl-ra", "0"},
			{"sec-ch-ua-mobile", "?0"},
			{"sec-ch-ua-model", `""`},
			{"sec-ch-ua-arch", `"x86"`},
			{"sec-ch-ua-full-version", secChUaVersion},
			{"user-agent", UserAgent},
			{"content-type", "text/plain;charset=UTF-8"},
			{"cf-chl", r.Headers["cf-chl"]},
			{"sec-ch-ua-platform-version", `"19.0.0"`},
			{"accept", "*/*"},
			{"origin", page.Scheme + "://" + page.Host},
			{"sec-fetch-site", "same-origin"},
			{"sec-fetch-mode", "cors"},
			{"sec-fetch-dest", "empty"},
			{"referer", pageURL},
			{"accept-encoding", acceptEncoding},
			{"accept-language", acceptLanguage},
			{"priority", "u=1, i"},
		}
	default: // challenge POST from the challenge iframe
		h = Headers{
			{"sec-ch-ua-platform", platform},
			{"user-agent", UserAgent},
			{"sec-ch-ua", secChUa},
			{"content-type", "text/plain;charset=UTF-8"},
			{"cf-chl", r.Headers["cf-chl"]},
			{"cf-chl-ra", "0"},
			{"sec-ch-ua-mobile", "?0"},
			{"accept", "*/*"},
			{"origin", target.Scheme + "://" + target.Host},
			{"sec-fetch-site", "same-origin"},
			{"sec-fetch-mode", "cors"},
			{"sec-fetch-dest", "empty"},
			{"sec-fetch-storage-access", "active"},
			{"referer", frameURL},
			{"accept-encoding", acceptEncoding},
			{"accept-language", acceptLanguage},
			{"priority", "u=1, i"},
		}
	}
	return h.With(r.Headers)
}

// NoCache puts pragma and cache-control (no-cache) first.
func (h Headers) NoCache() Headers {
	return append(Headers{{"pragma", "no-cache"}, {"cache-control", "no-cache"}}, h...)
}

// StorageAccess adds sec-fetch-storage-access after sec-fetch-dest, as Chrome
// sends it for requests to a cross-site frame's host.
func (h Headers) StorageAccess() Headers {
	i := slices.IndexFunc(h, func(p [2]string) bool { return p[0] == "sec-fetch-dest" }) + 1
	return slices.Insert(slices.Clone(h), i, [2]string{"sec-fetch-storage-access", "active"})
}

// With sets each extra header: a name already in h keeps its place, new
// names follow in name order.
func (h Headers) With(extra map[string]string) Headers {
	out := slices.Clone(h)
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		i := slices.IndexFunc(out, func(p [2]string) bool { return p[0] == strings.ToLower(name) })
		if i < 0 {
			out = append(out, [2]string{strings.ToLower(name), extra[name]})
		} else {
			out[i][1] = extra[name]
		}
	}
	return out
}
