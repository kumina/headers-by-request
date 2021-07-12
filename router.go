package headers_by_request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"sort"
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



type Header struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	ServiceId int    `json:"service_id"`
	Value     string `json:"value"`
}

type Rewrite struct {
	Id int `json:"id"`
	Pattern string `json:"pattern"`
	ServiceId int `json:"service_id"`
	Template string `json:"template"`
	Weight int `json:"weight"`
}


type Requested struct {
	Payload struct{
		Headers []Header `json:"headers,omitempty"`
		Rewrites []Rewrite `json:"rewrites,omitempty"`
	} `json:"payload"`
}


func (a *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	startTime := time.Time{}
	if a.enableTiming {
		startTime = time.Now()
	}

	path := ""
	if req.URL.RawPath == "" {
		path = req.URL.Path
	} else {
		path = req.URL.RawPath
	}

	fullUrl := fmt.Sprintf("%s%s", req.URL.Host, req.URL.Path)

	log.Println(fmt.Sprintf("Resolving header for %s", fullUrl))

	requestBody, err := json.Marshal(map[string]string{
		"request": fullUrl,
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
		if resp.StatusCode == 409 {
			log.Println(fmt.Sprintf("Ambiguous request."))
			rw.WriteHeader(http.StatusNotFound)
			return
		}
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
	requested := &Requested{}
	err = json.Unmarshal(body, requested)
	if err != nil {
		log.Println("Could not unmarshal requests body.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	for _, header := range requested.Payload.Headers {
		log.Println(fmt.Sprintf("Setting header: %s : %s", header.Name, header.Value))
		req.Header.Set(header.Name, header.Value)
	}

	// We only apply one rewrite when there are multiple.
	//rewrite := getHeaviestRewrite(requested.Payload.Rewrites)

	rewrites := requested.Payload.Rewrites

	// Sorts rewrites based on weight, high weight first in slice
	sort.Slice(rewrites[:], func(i, j int) bool {
		return rewrites[i].Weight > rewrites[j].Weight
	})

	for _, rewrite := range rewrites {
		check, err := regexp.Compile(rewrite.Pattern)
		if err != nil {
			log.Println("Could not compile regex.")
			continue
		}

		t := addDollarSigns(rewrite.Template)

		if check.Match([]byte(path)) {
			newpath := check.ReplaceAll([]byte(path), t)
			req.URL.Path = string(newpath)
			break
		}
	}

	if a.enableTiming {
		timeDiff := time.Now().Sub(startTime)
		log.Println(fmt.Sprintf("%s took %s", a.name, timeDiff))
	}
	a.next.ServeHTTP(rw, req)
}

func addDollarSigns(template string) []byte {
	c := regexp.MustCompile(`{([^}]+)}`)
	//c := regexp.MustCompile(`\{`)
	//c.ReplaceAll([]byte(template), []byte("${"))
	return c.ReplaceAll([]byte(template), []byte("${${1}}"))
}
