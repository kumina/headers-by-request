package traefik_routing_plugin_test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kumina/traefik-routing-plugin"
)

type UrlResponse struct {
	Url string `json:"url,omitempty"`
}

func TestRouter(t *testing.T)  {
	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	body1, _ := json.Marshal(traefik_routing_plugin.HeadersRequested{
		Headers: map[string]string{
			"header1": "value1",
			"header2": "value2",
		},
	})

	body2, _ := json.Marshal(traefik_routing_plugin.HeadersRequested{
		Headers: map[string]string{
			"header3": "value3",
			"header4": "value4",
		},
	})


	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		urlresponse := &UrlResponse{}
		_ = json.Unmarshal(body, urlresponse)
		u, _:= url.Parse(urlresponse.Url)
		if u.Path == "" {
			w.Write(body1)
		} else {
			w.Write(body2)
		}
	}))

	defer ts.Close()
	mockServerURL := ts.URL
	traefik_routing_plugin.Client = ts.Client()

	cfg := traefik_routing_plugin.CreateConfig()
	cfg.UrlHeaderRequest = mockServerURL

	handler, err := traefik_routing_plugin.New(ctx, next, cfg, "headers-by-request")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	assertHeader(t, req, "header1", "value1")
	assertHeader(t, req, "header2", "value2")

	recorder = httptest.NewRecorder()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/asd", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	assertHeader(t, req, "header3", "value3")
	assertHeader(t, req, "header4", "value4")
}

func assertHeader(t *testing.T, req *http.Request, key, expected string) {
	t.Helper()

	if req.Header.Get(key) != expected {
		t.Errorf("invalid header value: %s", req.Header.Get(key))
	}
}
