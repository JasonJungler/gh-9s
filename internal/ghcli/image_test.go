package ghcli

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type fakeHTTP struct {
	reqs   []*http.Request
	status int
	body   []byte
	length int64
	err    error
}

func (f *fakeHTTP) install(t *testing.T) {
	t.Helper()
	oldDo, oldTok := httpDo, ghToken
	httpDo = func(r *http.Request) (*http.Response, error) {
		f.reqs = append(f.reqs, r)
		if f.err != nil {
			return nil, f.err
		}
		st := f.status
		if st == 0 {
			st = 200
		}
		return &http.Response{StatusCode: st, Body: io.NopCloser(bytes.NewReader(f.body)), ContentLength: f.length}, nil
	}
	ghToken = func(host string) (string, error) { return "tok-" + host, nil }
	t.Cleanup(func() { httpDo, ghToken = oldDo, oldTok })
}

func TestFetchImageSendsTheTokenOnlyToTheRepoHost(t *testing.T) {
	f := &fakeHTTP{body: pngBytes(t)}
	f.install(t)
	if _, err := FetchImage("github.com", "https://github.com/user-attachments/assets/x.png"); err != nil {
		t.Fatal(err)
	}
	if got := f.reqs[0].Header.Get("Authorization"); got != "Bearer tok-github.com" {
		t.Errorf("same host: Authorization = %q", got)
	}
	if _, err := FetchImage("github.com", "https://evil.example/p.png"); err != nil {
		t.Fatal(err)
	}
	if got := f.reqs[1].Header.Get("Authorization"); got != "" {
		t.Errorf("other host got the token: %q", got)
	}
	if _, err := FetchImage("ghe.corp", "https://ghe.corp/a/b.png"); err != nil {
		t.Fatal(err)
	}
	if got := f.reqs[2].Header.Get("Authorization"); got != "Bearer tok-ghe.corp" {
		t.Errorf("enterprise host: Authorization = %q", got)
	}
}

func TestFetchImageResolvesAndRefusesLinks(t *testing.T) {
	f := &fakeHTTP{body: pngBytes(t)}
	f.install(t)
	for in, want := range map[string]string{
		"/a/b.png":                 "https://github.com/a/b.png",
		"//cdn.example/p.png":      "https://cdn.example/p.png",
		"http://plain.example/p.g": "http://plain.example/p.g",
	} {
		f.reqs = nil
		if _, err := FetchImage("github.com", in); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got := f.reqs[0].URL.String(); got != want {
			t.Errorf("%s resolved to %s; want %s", in, got, want)
		}
	}
	for _, bad := range []string{"data:image/png;base64,AAAA", "file:///etc/passwd", "javascript:alert(1)", "", "ftp://x/y.png"} {
		f.reqs = nil
		if _, err := FetchImage("github.com", bad); err == nil || len(f.reqs) != 0 {
			t.Errorf("%q: err=%v requests=%d; want a refusal and no request", bad, err, len(f.reqs))
		}
	}
}

func TestFetchImageDecodesPNGJPEGGIF(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var j, g bytes.Buffer
	_ = jpeg.Encode(&j, img, nil)
	_ = gif.Encode(&g, img, nil)
	for name, body := range map[string][]byte{"png": pngBytes(t), "jpeg": j.Bytes(), "gif": g.Bytes()} {
		f := &fakeHTTP{body: body}
		f.install(t)
		got, err := FetchImage("github.com", "https://h/p")
		if err != nil || got.Bounds().Dx() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestFetchImageErrorsAreSentences(t *testing.T) {
	big := bytes.Repeat([]byte{0}, maxImageBytes+10)
	cases := []struct {
		name string
		f    *fakeHTTP
		want string
	}{
		{"too large by header", &fakeHTTP{body: []byte("x"), length: int64(maxImageBytes + 1)}, "too large"},
		{"too large by body", &fakeHTTP{body: big, length: -1}, "too large"},
		{"webp or garbage", &fakeHTTP{body: []byte("RIFF....WEBPVP8 not an image")}, "unsupported image type"},
		{"empty", &fakeHTTP{body: nil}, "empty"},
		{"not found", &fakeHTTP{status: 404, body: []byte("nope")}, "HTTP 404"},
		{"forbidden", &fakeHTTP{status: 403}, "download refused"},
		{"network", &fakeHTTP{err: errors.New("dial tcp: no route")}, "no route"},
	}
	for _, c := range cases {
		c.f.install(t)
		_, err := FetchImage("github.com", "https://h/p.png")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v; want it to mention %q", c.name, err, c.want)
		}
	}
}

func TestRedirectDoesNotForwardTheTokenToAnotherHost(t *testing.T) {
	first, _ := http.NewRequest("GET", "https://github.com/a.png", nil)
	next, _ := http.NewRequest("GET", "https://objects.example/a.png", nil)
	next.Header.Set("Authorization", "Bearer secret")
	if err := imageRedirect(next, []*http.Request{first}); err != nil {
		t.Fatal(err)
	}
	if next.Header.Get("Authorization") != "" {
		t.Error("the token followed a redirect to another host")
	}
	same, _ := http.NewRequest("GET", "https://github.com/b.png", nil)
	same.Header.Set("Authorization", "Bearer secret")
	_ = imageRedirect(same, []*http.Request{first})
	if same.Header.Get("Authorization") == "" {
		t.Error("a redirect within the same host should keep the token")
	}
	many := make([]*http.Request, 6)
	for i := range many {
		many[i] = first
	}
	if err := imageRedirect(same, many); err == nil {
		t.Error("too many redirects should stop")
	}
}
