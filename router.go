package headers_by_request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
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
	enableTiming     bool
	logger           *LogLine
}

type LogLine struct {
	Timestamp time.Time         `json:"timestamp"`
	Msg       string            `json:"msg,omitempty"`
	Level     string            `json:"level"`
	Details   map[string]string `json:"details,inline"`
}

func NewLog() *LogLine {
	return &LogLine{
		Timestamp: time.Now(),
		Level:     "",
		Msg:       "",
		Details:   map[string]string{},
	}
}

func (l *LogLine) Info(msg string) *LogLine {
	n := NewLog()
	n.Level = "info"
	n.Msg = msg
	return n
}

func (l *LogLine) Error(msg string) *LogLine {
	n := NewLog()
	n.Level = "error"
	n.Msg = msg
	return n
}

func (l *LogLine) Warn(msg string) *LogLine {
	n := NewLog()
	n.Level = "warning"
	n.Msg = msg
	return n
}

func (l *LogLine) LogJson(values map[string]string) {
	for k, v := range values {
		l.Details[k] = v
	}
	l.Timestamp = time.Now()
	out, _ := json.Marshal(l)
	fmt.Println(string(out))
}

// Function needed for Traefik to recognize this module as a plugin
// Uses a generic http.Handler type from golang that we can use to work with the request
// by overriding different functions of the interface
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Log as JSON instead of the default ASCII formatter.
	logger := NewLog()

	logger.Info("New middleware created.").LogJson(nil)

	if len(config.UrlHeaderRequest) == 0 {
		return nil, fmt.Errorf("DynamicHeaderUrl cannot be empty")
	}
	Client = &http.Client{
		Timeout: 30 * time.Second,
	}

	return &Router{
		dynamicHeaderUrl: config.UrlHeaderRequest,
		enableTiming:     config.EnableTiming,
		next:             next,
		name:             name,
		logger:           logger,
	}, nil
}

type Header struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	ServiceId int    `json:"service_id"`
	Value     string `json:"value"`
}

type Rewrite struct {
	Id        int    `json:"id"`
	Pattern   string `json:"pattern"`
	ServiceId int    `json:"service_id"`
	Template  string `json:"template"`
	Weight    int    `json:"weight"`
}

type Requested struct {
	Payload struct {
		Headers  []Header  `json:"headers,omitempty"`
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

	a.logger.Info("Resolving route.").LogJson(map[string]string{"url": fullUrl})

	requestBody, err := json.Marshal(map[string]string{
		"request": fullUrl,
	})
	if err != nil {
		a.logger.Error("Requestbody marshalling error.").LogJson(map[string]string{"url": fullUrl})
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	resp, err := Client.Post(a.dynamicHeaderUrl, "application/json", bytes.NewBuffer(requestBody))

	if err != nil {
		//a.log.Error("Request error.")
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	if resp.StatusCode != 200 {
		if resp.StatusCode == 409 {
			a.logger.Info("Ambiguous request.").LogJson(map[string]string{"url": fullUrl})
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Info("Unknown status code response from DynamicHeaderUrl.").
			LogJson(map[string]string{"url": fullUrl, "code": strconv.Itoa(resp.StatusCode)})
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		a.logger.Info("Could not read requests body.").LogJson(map[string]string{"url": fullUrl})
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	requested := &Requested{}
	err = json.Unmarshal(body, requested)
	if err != nil {
		a.logger.Info("Could not unmarshal requests body.").LogJson(map[string]string{"url": fullUrl})
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	for _, header := range requested.Payload.Headers {

		a.logger.Info("Setting header.").LogJson(map[string]string{"url": fullUrl, "header_key": header.Name,
			"header_value": header.Value})
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
			a.logger.Warn("Could not compile regex.").LogJson(map[string]string{"url": fullUrl,
				"pattern": rewrite.Pattern})
			continue
		}

		t := addDollarSigns(rewrite.Template)

		if check.Match([]byte(path)) {
			newpath := check.ReplaceAll([]byte(path), t)
			req.URL.Path, err = url.PathUnescape(string(newpath))
			if err != nil {
				//a.log.WithField("path", newpath).Warn("Could not rewrite Path.")
				continue
			}
			a.logger.Warn("Apply rewrite.").LogJson(map[string]string{"url": fullUrl,
				"old_path": path, "new_path": string(newpath)})
			req.RequestURI = req.URL.RequestURI()
			break
		}
	}

	if a.enableTiming {
		timeDiff := time.Now().Sub(startTime)
		a.logger.Info("Resolving time.").LogJson(map[string]string{"url": fullUrl, "duration": strconv.FormatInt(timeDiff.Nanoseconds(), 10)})
	}

	a.next.ServeHTTP(rw, req)
}

func addDollarSigns(template string) []byte {
	c := regexp.MustCompile(`{([^}]+)}`)
	//c := regexp.MustCompile(`\{`)
	//c.ReplaceAll([]byte(template), []byte("${"))
	return c.ReplaceAll([]byte(template), []byte("${${1}}"))
}
