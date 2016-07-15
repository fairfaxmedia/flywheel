package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// Handler flywheel handler
type Handler struct {
	flywheel *Flywheel
	tmpl     *template.Template
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

	handler.flywheel.pings <- sreq
	status := <-replyTo
	if err != nil && status.Err == nil {
		status.Err = err
	}
	return status
}

func (handler *Handler) proxy(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}
	r.URL.Query().Del("flywheel")

	endpoint := handler.flywheel.ProxyEndpoint(r.Host)
	if endpoint == "" {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Invalid flywheel endpoint config"))
		log.Fatal("Invalid endpoint URL")
	}

	r.URL.Scheme = "http"

	r.URL.Host = endpoint
	r.RequestURI = ""
	resp, err := client.Do(r)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	for key, value := range resp.Header {
		w.Header()[key] = value
	}
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Print(err)
	}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] %s %s", r.RemoteAddr, r.Method, r.RequestURI)

	query := r.URL.Query()
	param := query.Get("flywheel")

	if param == "config" {
		buf, err := json.MarshalIndent(handler.flywheel.config, "", "    ") // Might be unsafe, but this should be read only.
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
