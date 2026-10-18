package onestop

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type identity struct {
	Site           string `json:"site,omitempty"`
	UserAgent      string `json:"userAgent,omitempty"`
	Referer        string `json:"referer,omitempty"`
	AcceptLanguage string `json:"acceptLanguage,omitempty"`
	IP             string `json:"ip,omitempty"`
}

type datadomeIdentity struct {
	Site           string `json:"site,omitempty"`
	UserAgent      string `json:"userAgent,omitempty"`
	AcceptLanguage string `json:"acceptLanguage,omitempty"`
	IP             string `json:"ip,omitempty"`
	Referer        string `json:"referer,omitempty"`
}

type refererIdentity struct {
	Referer        string `json:"referer,omitempty"`
	Site           string `json:"site,omitempty"`
	UserAgent      string `json:"userAgent,omitempty"`
	AcceptLanguage string `json:"acceptLanguage,omitempty"`
	IP             string `json:"ip,omitempty"`
}

type akamaiWire struct {
	Referer        string `json:"referer,omitempty"`
	Site           string `json:"site,omitempty"`
	Abck           string `json:"abck"`
	Bmsz           string `json:"bmsz"`
	UserAgent      string `json:"userAgent,omitempty"`
	AcceptLanguage string `json:"acceptLanguage,omitempty"`
	IP             string `json:"ip,omitempty"`
	Script         string `json:"script,omitempty"`
	ScriptURL      string `json:"scriptUrl"`
	Context        string `json:"context,omitempty"`
}

type sbsdWire struct {
	refererIdentity
	UUID       string `json:"uuid"`
	SbsdCookie string `json:"sbsdCookie"`
	Script     string `json:"script,omitempty"`
	Mode       string `json:"mode"`
	Context    string `json:"context,omitempty"`
}

type pixelWire struct {
	identity
	PixelCall
}

type datadomeTagsWire struct {
	datadomeIdentity
	DatadomeTagsCall
}

type datadomeInterstitialWire struct {
	datadomeIdentity
	DatadomeInterstitialCall
}

type datadomeSliderWire struct {
	datadomeIdentity
	DatadomeSliderCall
}

type cloudflareWire struct {
	identity
	CloudflareCall
}

type kasadaPScriptWire struct {
	Referer        string `json:"referer"`
	Site           string `json:"site"`
	UserAgent      string `json:"userAgent"`
	AcceptLanguage string `json:"acceptLanguage"`
	IP             string `json:"ip"`
	Script         string `json:"script"`
}

type kasadaTLWire struct {
	refererIdentity
	IPSURL string `json:"ipsUrl"`
	Script string `json:"script"`
}

type impervaWire struct {
	datadomeIdentity
	ScriptURL string `json:"scriptUrl"`
	Script    string `json:"script"`
}

type envelope struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type response[T any] interface {
	*T
	meta() *Meta
}

func generate[T any, PT response[T]](s Session, mode, provider string, request any) (*T, error) {
	c := s.client
	if c == nil {
		return nil, errors.New("onestop: use Client.Session(...) before challenge calls")
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string]any{provider + "Request": request}); err != nil {
		return nil, fmt.Errorf("onestop: encode request: %w", err)
	}
	data := body.Bytes()
	switch c.compression {
	case "zstd":
		data = c.encoder.EncodeAll(data, nil)
	case "gzip":
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		w.Write(data)
		w.Close()
		data = b.Bytes()
	}
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("onestop: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if c.compression != "identity" {
		req.Header.Set("Content-Encoding", c.compression)
	}
	req.Header.Set("Accept-Encoding", "zstd, gzip")
	req.Header.Set("x-mode", mode)
	c.setAuthHeaders(req)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("onestop: request: %w", err)
	}
	defer res.Body.Close()
	requestID := res.Header.Get("RequestID")
	data, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("onestop: read response: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		var r *gzip.Reader
		r, err = gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			data, err = io.ReadAll(r)
		}
	case "zstd":
		d, _ := zstd.NewReader(nil)
		data, err = d.DecodeAll(data, nil)
		d.Close()
	default:
		return nil, fmt.Errorf("onestop: unsupported response compression %q", res.Header.Get("Content-Encoding"))
	}
	if err != nil {
		return nil, fmt.Errorf("onestop: decompress response: %w", err)
	}
	var env envelope
	var fields map[string]json.RawMessage
	result := new(T)
	err = json.Unmarshal(data, &env)
	if err == nil {
		err = json.Unmarshal(data, &fields)
	}
	if raw, ok := fields[provider+"Response"]; ok && err == nil {
		err = json.Unmarshal(raw, result)
	}
	if err != nil {
		return nil, &APIError{StatusCode: res.StatusCode, RequestID: requestID, Message: "invalid JSON response"}
	}
	if res.StatusCode != http.StatusOK || !env.Success || env.Error != "" {
		message := env.Error
		if message == "" {
			message = "request failed"
		}
		return nil, &APIError{StatusCode: res.StatusCode, RequestID: requestID, Message: message}
	}
	meta := PT(result).meta()
	meta.RequestID = requestID
	if meta.Sleep > 0 {
		timer := time.NewTimer(time.Duration(meta.Sleep) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-c.ctx.Done():
			return nil, fmt.Errorf("onestop: request: %w", c.ctx.Err())
		case <-timer.C:
		}
	}
	return result, nil
}
