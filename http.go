package flywheel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"
)

// Handler flywheel handler
type Handler struct {
	Flywheel *Flywheel
	tmpl     *template.Template

	// HTTPClient is the HTTP client to use when proxying request to the backends
	// This is used to control redirect behavior.
	HTTPClient *http.Client
}

// ErrIgnoreRedirects used for proxy redirect ignore
var ErrIgnoreRedirects = errors.New("Ignore Redirect Error")

// NewHandler create flywheel http handler
func NewHandler(fw *Flywheel) *Handler {
	return &Handler{
		Flywheel: fw,
		HTTPClient: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return ErrIgnoreRedirects
			},
		},
	}
}

// sendPing - sends a request to the flywheel to retrieve/change the state
func (handler *Handler) sendPing(op string) Pong {
	var err error

	replyTo := make(chan Pong, 1)
	sreq := Ping{replyTo: replyTo}
	switch op {
	case "start":
		sreq.requestStart = true
	case "stop":
		sreq.requestStop = true
	case "status":
		sreq.noop = true
	}
	if strings.HasPrefix(op, "stop_in:") {
		suffix := op[8:]
		dur, e := time.ParseDuration(suffix)
		if e != nil {
			err = e
		}
		sreq.setTimeout = dur
	}

	handler.Flywheel.pings <- sreq
	status := <-replyTo
	if err != nil && status.Err == nil {
		status.Err = err
	}
	return status
}

// TODO - refactor this function to use context
// TODO - add support for SSL
func (handler *Handler) proxy(w http.ResponseWriter, r *http.Request) {

	r.URL.Host = handler.Flywheel.ProxyEndpoint(r.Host)
	r.URL.Scheme = "http"
	r.RequestURI = ""
	r.URL.Query().Del("flywheel")

	resp, err := handler.HTTPClient.Do(r)

	if err != nil {
		if urlError, ok := err.(*url.Error); ok && urlError.Err == ErrIgnoreRedirects {
			err = nil
		} else {
			log.Print(err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}

	for key, value := range resp.Header {
		w.Header()[key] = value
	}
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	// if response code is between 300 and 400 sometimes body does not exist
	if err != nil && (!(resp.StatusCode >= 300 && resp.StatusCode < 400)) {
		log.Print(err)
	}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] %s %s", r.RemoteAddr, r.Method, r.RequestURI)

	query := r.URL.Query()
	param := query.Get("flywheel")

	if param == "config" {
		buf, err := json.MarshalIndent(handler.Flywheel.config, "", "    ") // Might be unsafe, but this should be read only.
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write(buf)
		}
		return
	}

	pong := handler.sendPing(param)

	if param == "start" {
		query.Del("flywheel")
		r.URL.RawQuery = query.Encode()
		w.Header().Set("Location", r.URL.String())
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}

	accept := query.Get("Accept")
	var acceptHTML bool
	if accept != "" {
		htmlIndex := strings.Index(accept, "text/html")
		jsonIndex := strings.Index(accept, "application/json")
		if htmlIndex != -1 {
			acceptHTML = jsonIndex == -1 || htmlIndex < jsonIndex
		}
	}

	if param != "" {
		buf, err := json.MarshalIndent(pong, "", "    ")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}

		if param != "status" {
			query.Del("flywheel")
			r.URL.RawQuery = query.Encode()
			w.Header().Set("Content-Type", "application/json")
			if acceptHTML {
				w.Header().Set("Location", r.URL.String())
				w.WriteHeader(http.StatusTemporaryRedirect)
			}
			w.Write(buf)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write(buf)
		}
		return
	}

	if pong.Err != nil {
		body := fmt.Sprintf(HTMLERROR, pong.Err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(body))
		return
	}

	switch pong.Status {
	case STOPPED:
		query.Set("flywheel", "start")
		r.URL.RawQuery = query.Encode()
		body := fmt.Sprintf(HTMLSTOPPED, r.URL)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(body))
	case STARTING:
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(HTMLSTARTING))
	case STARTED:
		handler.proxy(w, r)
	case STOPPING:
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(HTMLSTOPPING))
	case UNHEALTHY:
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(HTMLUNHEALTHY))
	}
}
