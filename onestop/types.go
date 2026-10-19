package onestop

// AkamaiSensorCall holds the inputs for AkamaiSensor. Send Script on the
// first call and Context, without Script, on every later call.
type AkamaiSensorCall struct {
	Abck, Bmsz, Script, ScriptURL, Context string
}

// SbsdCall holds the inputs for SbsdPayload. Mode is "f" (fingerprint, with
// Script) or "t" (event, with Context).
type SbsdCall struct {
	UUID, SbsdCookie, Script, Mode, Context string
}

// PixelCall holds the raw page HTML and Pixel script.
type PixelCall struct {
	HTML    string `json:"html"`
	Script  string `json:"script"`
	Context string `json:"context,omitempty"`
}

// BmpCall describes Android/iOS SDK inputs; no browser identity is sent.
type BmpCall struct {
	BundleID         string            `json:"bundleId"`
	Version          string            `json:"version"`
	Device           string            `json:"device"`
	Nonce            string            `json:"nonce,omitempty"`
	Checksum         string            `json:"checksum,omitempty"`
	ServerSideSignal string            `json:"serverSideSignal,omitempty"`
	Token            string            `json:"token,omitempty"`
	Difficulty       *int              `json:"difficulty,omitempty"`
	IOSVersion       string            `json:"iosVersion,omitempty"`
	UUID             string            `json:"uuid,omitempty"`
	RealValues       map[string]string `json:"realValues,omitempty"`
	Script           string            `json:"script,omitempty"`
}

// DatadomeTagsCall holds the inputs for DatadomeTags. Event is "ch" or "le".
type DatadomeTagsCall struct {
	Event   string `json:"event"`
	SiteKey string `json:"siteKey"`
	Version string `json:"version"`
	Cookie  string `json:"cookie"`
	Context string `json:"context,omitempty"`
}

type DatadomeInterstitialCall struct {
	Script       string `json:"script"`
	HTML         string `json:"html"`
	PageResponse string `json:"pageResponse"`
	Context      string `json:"context,omitempty"`
}

// DatadomeSliderCall holds the inputs for DatadomeSlider. BackgroundImage and
// PuzzlePiece are base64.
type DatadomeSliderCall struct {
	Script          string `json:"script"`
	HTML            string `json:"html"`
	PageResponse    string `json:"pageResponse"`
	BackgroundImage string `json:"backgroundImage"`
	PuzzlePiece     string `json:"puzzlePiece"`
	Context         string `json:"context,omitempty"`
}

// CloudflareCall holds the inputs for the Cloudflare methods. Response
// accepts a string, a JSON object or json.RawMessage.
type CloudflareCall struct {
	HTML          string `json:"html,omitempty"`
	Script        string `json:"script,omitempty"`
	ScriptURL     string `json:"scriptUrl,omitempty"`
	Response      any    `json:"response,omitempty"`
	Context       string `json:"context,omitempty"`
	RayID         string `json:"rayId,omitempty"`
	BytecodeRayID string `json:"bytecodeRayId,omitempty"`
	IframeURL     string `json:"iframeUrl,omitempty"`
	ImageHeight   *int   `json:"imageHeight,omitempty"`
	ImageWidth    *int   `json:"imageWidth,omitempty"`
	PatStatus     *int   `json:"patStatus,omitempty"`
	Action        string `json:"action,omitempty"`
	CData         string `json:"cData,omitempty"`
}

// ImpervaSessionCookie is one incap_ses_* cookie of the page host, with its
// exact value, for ImpervaUtmvc.
type ImpervaSessionCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AkamaiResponse is returned by AkamaiSensor, Pixel and Bmp.
type AkamaiResponse struct {
	Payload string `json:"payload"`
	Context string `json:"context"`
	Meta
}

// PixelResponse.Payload is the complete form body for the Pixel POST.
type PixelResponse = AkamaiResponse

type BmpResponse = AkamaiResponse

type SbsdResponse struct {
	Payload string `json:"payload"`
	Context string `json:"context"`
	Meta
}

// DatadomeResponse is returned by the DataDome methods. Payload is already
// encoded.
type DatadomeResponse struct {
	Payload string `json:"payload"`
	Context string `json:"context"`
	Meta
}

type (
	DatadomeTagsResponse         = DatadomeResponse
	DatadomeInterstitialResponse = DatadomeResponse
	DatadomeSliderResponse       = DatadomeResponse
)

// CloudflareResponse lists the target requests for one challenge step.
type CloudflareResponse struct {
	Requests []CloudflareHTTPRequest `json:"requests"`
	Token    string                  `json:"token"`
	Context  string                  `json:"context"`
	Meta
}

// CloudflareHTTPRequest is one target request; Purpose is challenge, iframe,
// image, pat or form.
type CloudflareHTTPRequest struct {
	Purpose string            `json:"purpose"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// KasadaPScriptResponse holds values extracted from p.js; the crypto fields
// feed KasadaCDHeader.
type KasadaPScriptResponse struct {
	Version              string            `json:"version"`
	Headers              map[string]string `json:"headers"`
	Difficulty           int               `json:"difficulty"`
	SubchallengeCount    int               `json:"subchallengeCount"`
	SeedSuffix           string            `json:"seedSuffix"`
	CryptoCustomerKey    string            `json:"cryptoCustomerKey"`
	CacheLifetimeSeconds int               `json:"cacheLifetimeSeconds"`
	Meta
}

// KasadaResponse holds the base64 TL body and the headers to send with it.
type KasadaResponse struct {
	Payload string            `json:"payload"`
	Headers map[string]string `json:"headers"`
	Meta
}

// ImpervaResponse holds the reese84 POST body (JSON text), or the ___utmvc
// cookie value; send it as is.
type ImpervaResponse struct {
	Payload string `json:"payload"`
	Meta
}

// Meta is embedded in every result. RequestID identifies the call for support.
// Sleep is the wait in milliseconds the API asked for; the SDK has already
// waited it before returning the result.
type Meta struct {
	RequestID string `json:"-"`
	Sleep     int    `json:"sleep"`
}

func (m *Meta) meta() *Meta { return m }
