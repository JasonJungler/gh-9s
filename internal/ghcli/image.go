package ghcli

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // decoders for image.Decode
	_ "image/jpeg" // decoders for image.Decode
	_ "image/png"  // decoders for image.Decode
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxImageBytes  = 5 << 20
	maxImagePixels = 25_000_000 // decoded, an image takes 4 bytes a pixel
)

var imageClient = &http.Client{Timeout: 20 * time.Second, CheckRedirect: imageRedirect}

// Seams: tests never reach the network or run gh.
var (
	httpDo  = func(r *http.Request) (*http.Response, error) { return imageClient.Do(r) }
	ghToken = func(host string) (string, error) {
		out, err := Run("auth", "token", "--hostname", host)
		return strings.TrimSpace(string(out)), err
	}
)

// imageRedirect follows up to five redirects and takes the token off any request that leaves the
// host the download started on.
func imageRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Hostname() != via[0].URL.Hostname() {
		req.Header.Del("Authorization")
	}
	return nil
}

// FetchImage downloads and decodes a png, jpeg or gif. host is the GitHub host of the repo the
// text came from; the gh token is sent only to that host.
func FetchImage(host, rawURL string) (image.Image, error) {
	if host == "" {
		host = "github.com"
	}
	u, err := resolveImageURL(host, rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "https" && u.Hostname() == host && (u.Port() == "" || u.Port() == "443") {
		if tok, err := ghToken(host); err == nil && tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := httpDo(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download refused (HTTP %d)", resp.StatusCode)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, errors.New("image is too large (> 5 MB)")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, errors.New("image is too large (> 5 MB)")
	}
	if len(data) == 0 {
		return nil, errors.New("the image is empty")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("unsupported image type (only png, jpeg and gif can be shown)")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, fmt.Errorf("image is too large (%d×%d pixels)", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("unsupported image type (only png, jpeg and gif can be shown)")
	}
	return img, nil
}

// ResolveImageURL is the absolute http(s) link a reference in some text stands for; anything else
// (files, other schemes, option-looking strings) is refused. Only this value may be opened.
func ResolveImageURL(host, raw string) (string, error) {
	if host == "" {
		host = "github.com"
	}
	u, err := resolveImageURL(host, raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func resolveImageURL(host, raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || raw == "" {
		return nil, errors.New("not a usable image link")
	}
	switch {
	case u.Scheme == "" && u.Host != "": // //cdn.example/p.png
		u.Scheme = "https"
	case u.Scheme == "" && strings.HasPrefix(u.Path, "/"):
		u.Scheme, u.Host = "https", host
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("not a usable image link (only http and https links are fetched)")
	}
	return u, nil
}
