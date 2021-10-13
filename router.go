package headers_by_request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
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
	log *log.Entry
}

// Function needed for Traefik to recognize this module as a plugin
// Uses a generic http.Handler type from golang that we can use to work with the request
// by overriding different functions of the interface
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	log.SetOutput(os.Stdout)

	logcontext := log.WithFields(log.Fields{
		"middleware": name,
	})
	logcontext.Info("New middleware created.")

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
		log: logcontext,
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

	// https://github.com/traefik/traefik/blob/817ac8f256a3ccf89801072b957d7d9f28f8c2f3/pkg/middlewares/redirect/redirect.go#L102

	fullUrl := fmt.Sprintf("%s%s", req.URL.Host, req.URL.Path)

	a.log.WithField("url", fullUrl).Info("Resolving route")

	requestBody, err := json.Marshal(map[string]string{
		"request": fullUrl,
	})
	if err != nil {
		a.log.Error("Requestbody marshalling error.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	resp, err := Client.Post(a.dynamicHeaderUrl, "application/json", bytes.NewBuffer(requestBody))

	if err != nil {
		a.log.Error("Request error.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	if resp.StatusCode != 200 {
		if resp.StatusCode == 409 {
			a.log.WithField("url", fullUrl).Info(fmt.Sprintf("Ambiguous request."))
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		a.log.WithField("code", resp.StatusCode).Error(fmt.Sprintf("Unknown statuscode."))
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		a.log.Error("Could not read requests body.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	requested := &Requested{}
	err = json.Unmarshal(body, requested)
	if err != nil {
		a.log.Error("Could not unmarshal requests body.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	for _, header := range requested.Payload.Headers {
		a.log.WithField(header.Name, header.Value).Info("Setting header.")
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
			a.log.WithField("pattern", rewrite.Pattern).Warn("Could not compile regex.")
			continue
		}

		t := addDollarSigns(rewrite.Template)

		if check.Match([]byte(path)) {
			newpath := check.ReplaceAll([]byte(path), t)
			req.URL.Path, err = url.PathUnescape(string(newpath))
			if err != nil {
				a.log.WithField("path", newpath).Warn("Could not rewrite Path.")
				continue
			}
			a.log.WithField("old_path", path).WithField("new_path", string(newpath)).Info("Apply rewrite.")
			req.RequestURI = req.URL.RequestURI()
			break
		}
	}

	if a.enableTiming {
		timeDiff := time.Now().Sub(startTime)
		a.log.WithField("duration", timeDiff.String()).Info()
	}

	a.next.ServeHTTP(rw, req)
}

func addDollarSigns(template string) []byte {
	c := regexp.MustCompile(`{([^}]+)}`)
	//c := regexp.MustCompile(`\{`)
	//c.ReplaceAll([]byte(template), []byte("${"))
	return c.ReplaceAll([]byte(template), []byte("${${1}}"))
}
