package headers_by_request_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kumina/headers-by-request"
)

type UrlResponse struct {
	Request string `json:"request,omitempty"`
}

func TestRouter(t *testing.T)  {
	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	r1 := headers_by_request.Requested{}

	r1.Payload.Headers = []headers_by_request.Header{{
		Name:      "header1",
		Id:        1,
		ServiceId: 1,
		Value:     "value1",
	},
	{
		Name:      "header2",
		Id:        2,
		ServiceId: 2,
		Value:     "value2",
	},
	}

	body1, _ := json.Marshal(r1)

	r2 := headers_by_request.Requested{}

	r2.Payload.Rewrites = []headers_by_request.Rewrite{{
		Pattern:      "/test2/path/(?P<test_var>.*)/(?P<another>.*)",
		Id:        1,
		ServiceId: 1,
		Template:     "/new/{test_var}/more/{another}",
		Weight: 100,
	},
	{
		Pattern:      "/test2/foo/bar/(?P<asd>.*)",
		Id:        2,
		ServiceId: 2,
		Template:     "/zoo/{asd}",
		Weight: 90,
	},
	}

	body2, _ := json.Marshal(r2)

	r3 := headers_by_request.Requested{}

	r3.Payload.Rewrites = []headers_by_request.Rewrite{{
			Pattern:   "/test3/notmatching/path/(?P<test_var>.*)",
			Id:        1,
			ServiceId: 1,
			Template:  "/new/{test_var}",
			Weight:    200,
		},
		{
			Pattern:   "/test3/foo/bar/(?P<test_var>.*)",
			Id:        2,
			ServiceId: 2,
			Template:  "/somethingelse/{test_var}",
			Weight:    80,
		},
		{
			Pattern:   "/test3/foo/bar/(?P<asd>.*)",
			Id:        3,
			ServiceId: 3,
			Template:  "/zoo/{asd}",
			Weight:    100,
		},
	}

	body3, _ := json.Marshal(r3)

	r4 := struct {
		Payload struct{
			Message string `json:"message"`
			Request string `json:"request"`
		} `json:"payload"`
	}{}

	r4.Payload.Message = "Ambiguous request"
	r4.Payload.Request = "request.url/test"

	body4, err := json.Marshal(r4)


	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		urlresponse := &UrlResponse{}
		_ = json.Unmarshal(body, urlresponse)
		u, _:= url.Parse(fmt.Sprintf("http://%s", urlresponse.Request))
		if u.Path == "/test1" {
			w.Write(body1)
		} else if u.Path == "/test2/path/rewritepart/bla" {
			w.Write(body2)
		} else if u.Path == "/test3/foo/bar/weight" {
			w.Write(body3)
		} else if u.Path == "/test4/ambiguous" {
			w.WriteHeader(409)
			w.Write(body4)
		} else {
			w.Write([]byte(""))
		}
	}))

	defer ts.Close()
	mockServerURL := ts.URL
	headers_by_request.Client = ts.Client()

	cfg := headers_by_request.CreateConfig()
	cfg.UrlHeaderRequest = mockServerURL
	cfg.EnableTiming = true

	handler, err := headers_by_request.New(ctx, next, cfg, "headers-by-request")
	if err != nil {
		t.Fatal(err)
	}

	// test1 testing headers
	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/test1", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	assertHeader(t, req, "header1", "value1")
	assertHeader(t, req, "header2", "value2")
	assertCode(t, recorder, 200)

	// test2 testing rewrites
	recorder = httptest.NewRecorder()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/test2/path/rewritepart/bla", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	assertUrl(t, req, "http://localhost/new/rewritepart/more/bla")
	assertCode(t, recorder, 200)

	// test3 test rewrite weight
	recorder = httptest.NewRecorder()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/test3/foo/bar/weight", nil)

	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	assertUrl(t, req, "http://localhost/zoo/weight")
	assertCode(t, recorder, 200)

	// test4 test ambiguous response
	recorder = httptest.NewRecorder()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/test4/ambiguous", nil)

	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	assertCode(t, recorder, 404)
	fmt.Println(req.RequestURI)
}

func assertHeader(t *testing.T, req *http.Request, key, expected string) {
	t.Helper()

	if req.Header.Get(key) != expected {
		t.Errorf("invalid header value: %s != %s", req.Header.Get(key), expected)
	}
}

func assertUrl(t *testing.T, req *http.Request, expected string) {
	t.Helper()

	if req.URL.String() != expected {
		t.Errorf("invalid URL value: %s != %s", req.URL.String(), expected)
	}
}

func assertCode(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()

	if rec.Code != expected {
		t.Errorf("invalid statuscode %d != %d", rec.Code, expected)
	}
}