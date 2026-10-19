package onestop

import "errors"

// Session is the browser identity sent with every challenge call. Create it
// with Client.Session; a zero Session is only valid as Config.Defaults or
// Client.Session input.
type Session struct {
	client *Client

	UserAgent, AcceptLanguage, IP string
	Site, Referer                 string // Absolute page URL and optional HTTP referer.
}

func (s Session) identity() identity {
	return identity{s.Site, s.UserAgent, s.Referer, s.AcceptLanguage, s.IP}
}

func (s Session) datadomeIdentity() datadomeIdentity {
	return datadomeIdentity{s.Site, s.UserAgent, s.AcceptLanguage, s.IP, s.Referer}
}

func (s Session) refererIdentity() refererIdentity {
	return refererIdentity{s.Referer, s.Site, s.UserAgent, s.AcceptLanguage, s.IP}
}

func (s Session) AkamaiSensor(call AkamaiSensorCall) (*AkamaiResponse, error) {
	return generate[AkamaiResponse](s, "Akamai", "akamai", akamaiWire{
		s.Referer, s.Site, call.Abck, call.Bmsz, s.UserAgent, s.AcceptLanguage, s.IP, call.Script, call.ScriptURL, call.Context,
	})
}

func (s Session) SbsdPayload(call SbsdCall) (*SbsdResponse, error) {
	return generate[SbsdResponse](s, "Sbsd", "sbsd", sbsdWire{s.refererIdentity(), call.UUID, call.SbsdCookie, call.Script, call.Mode, call.Context})
}

func (s Session) Pixel(call PixelCall) (*PixelResponse, error) {
	return generate[PixelResponse](s, "Akamai", "pixel", pixelWire{s.identity(), call})
}

// Bmp generates an Akamai BMP (mobile SDK) payload.
func (s Session) Bmp(call BmpCall) (*BmpResponse, error) {
	return generate[BmpResponse](s, "Akamai", "bmp", call)
}

func (s Session) DatadomeTags(call DatadomeTagsCall) (*DatadomeTagsResponse, error) {
	return generate[DatadomeTagsResponse](s, "DatadomeTags", "datadomeTags", datadomeTagsWire{s.datadomeIdentity(), call})
}

func (s Session) DatadomeInterstitial(call DatadomeInterstitialCall) (*DatadomeInterstitialResponse, error) {
	return generate[DatadomeInterstitialResponse](s, "DatadomeInterstitial", "datadomeInterstitial", datadomeInterstitialWire{s.datadomeIdentity(), call})
}

// DatadomeSlider returns the check request's query string as Payload.
func (s Session) DatadomeSlider(call DatadomeSliderCall) (*DatadomeSliderResponse, error) {
	return generate[DatadomeSliderResponse](s, "DatadomeSlider", "datadomeSlider", datadomeSliderWire{s.datadomeIdentity(), call})
}

// CloudflareStep runs one Cloudflare step; the API detects the challenge type.
func (s Session) CloudflareStep(call CloudflareCall) (*CloudflareResponse, error) {
	return s.cloudflare("Cloudflare", call)
}

func (s Session) CloudflareTurnstile(call CloudflareCall) (*CloudflareResponse, error) {
	return s.cloudflare("CloudflareTurnstile", call)
}

func (s Session) CloudflareWaf(call CloudflareCall) (*CloudflareResponse, error) {
	return s.cloudflare("CloudflareWaf", call)
}

func (s Session) CloudflareJsd(call CloudflareCall) (*CloudflareResponse, error) {
	return s.cloudflare("CloudflareJsd", call)
}

func (s Session) CloudflarePrecursor(call CloudflareCall) (*CloudflareResponse, error) {
	return s.cloudflare("CloudflarePrecursor", call)
}

func (s Session) cloudflare(mode string, call CloudflareCall) (*CloudflareResponse, error) {
	return generate[CloudflareResponse](s, mode, "cloudflare", cloudflareWire{s.identity(), call})
}

// KasadaPScript extracts the version and CD inputs from raw p.js source.
func (s Session) KasadaPScript(script string) (*KasadaPScriptResponse, error) {
	return generate[KasadaPScriptResponse](s, "Kasada", "kasadaPscript", kasadaPScriptWire{s.Referer, s.Site, s.UserAgent, s.AcceptLanguage, s.IP, script})
}

// KasadaTL generates the TL body from the ips.js URL and raw source.
func (s Session) KasadaTL(scriptURL, script string) (*KasadaResponse, error) {
	return generate[KasadaResponse](s, "Kasada", "kasada", kasadaTLWire{s.refererIdentity(), scriptURL, script})
}

// Imperva generates the reese84 POST body from the challenge script's URL and raw source.
func (s Session) Imperva(scriptURL, script string) (*ImpervaResponse, error) {
	return generate[ImpervaResponse](s, "Imperva", "imperva", impervaWire{s.datadomeIdentity(), scriptURL, script})
}

// ImpervaUtmvc generates the ___utmvc cookie value from the utmvc script's URL
// and raw source and the page host's incap_ses_* cookies, in browser cookie order.
func (s Session) ImpervaUtmvc(scriptURL, script string, cookies []ImpervaSessionCookie) (*ImpervaResponse, error) {
	if len(cookies) == 0 {
		return nil, errors.New("onestop: cookies is required: the page's incap_ses_* cookies")
	}
	return generate[ImpervaResponse](s, "Imperva", "imperva", impervaUtmvcWire{s.datadomeIdentity(), scriptURL, script, cookies})
}
