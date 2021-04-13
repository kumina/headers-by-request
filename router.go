package headers_by_request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var (
	Client *http.Client
)

type Router struct {
	// Required by Traefik
	next http.Handler
	name string

	// Our custom configuration
	dynamicHeaderUrl string
	enableTiming bool
}

// Function needed for Traefik to recognize this module as a plugin
// Uses a generic http.Handler type from golang that we can use to work with the request
// by overriding different functions of the interface
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	log.Println("New middleware created.")

	// todo: check if the url is valid?
	if len(config.UrlHeaderRequest) == 0 {
		return nil, fmt.Errorf("DynamicHeaderUrl cannot be empty")
	}
	Client = &http.Client{
		Timeout: 30 * time.Second,
	}

	return &Router{
		dynamicHeaderUrl: config.UrlHeaderRequest,
		enableTiming: config.EnableTiming,
		next:   next,
		name:   name,
	}, nil
}

type HeadersRequested struct {
	Headers map[string]string `json:"headers,omitempty"`
}


func (a *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	startTime := time.Time{}
	if a.enableTiming {
		startTime = time.Now()
	}
	log.Println(fmt.Sprintf("Resolving header for %s", req.URL))

	requestBody, err := json.Marshal(map[string]string{
		"url": fmt.Sprintf("%s", req.URL),
	})
	if err != nil {
		log.Println("Requestbody marshalling error.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	resp, err := Client.Post(a.dynamicHeaderUrl, "application/json", bytes.NewBuffer(requestBody))

	if err != nil {
		log.Println("Request error.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	if resp.StatusCode != 200 {
		log.Println(fmt.Sprintf("Request return statuscode %d.", resp.StatusCode))
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Could not read requests body.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	headers := &HeadersRequested{}
	err = json.Unmarshal(body, headers)
	if err != nil {
		log.Println("Could not unmarshal requests body.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	for key, value := range headers.Headers {
		log.Println(fmt.Sprintf("Setting header: %s : %s", key, value))
		req.Header.Set(key, value)
	}

	if a.enableTiming {
		timeDiff := time.Now().Sub(startTime)
		log.Println(fmt.Sprintf("%s took %s", a.name, timeDiff))
	}
	a.next.ServeHTTP(rw, req)
}
