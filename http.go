package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

func (fw *Flywheel) SendPing(op string) Pong {
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

	fw.pings <- sreq

	status := <-replyTo
	return status
}

func (fw *Flywheel) ProxyEndpoint(hostname string) string {
	vhost, ok := fw.config.Vhosts[hostname]
	if ok {
		return vhost
	}
	return fw.config.Endpoint
}

func (fw *Flywheel) Proxy(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}
	r.URL.Query().Del("flywheel")

	endpoint := fw.ProxyEndpoint(r.Host)
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

func (fw *Flywheel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[%s] %s %s", r.RemoteAddr, r.Method, r.RequestURI)

	query := r.URL.Query()
	flywheel := query.Get("flywheel")

	if flywheel == "config" {
		buf, err := json.Marshal(fw.config) // Might be unsafe, but this should be read only.
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write(buf)
		}
		return
	}

	pong := fw.SendPing(query.Get("flywheel"))

	if flywheel == "start" {
		query.Del("flywheel")
		r.URL.RawQuery = query.Encode()
		w.Header().Set("Location", r.URL.String())
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}

	accept := query.Get("Accept")
	var acceptHtml bool
	if accept != "" {
		htmlIndex := strings.Index(accept, "text/html")
		jsonIndex := strings.Index(accept, "application/json")
		if htmlIndex != -1 {
			acceptHtml = jsonIndex == -1 || htmlIndex < jsonIndex
		}
	}

	if flywheel != "" {
		buf, err := json.Marshal(pong)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
		} else if flywheel != "status" {
			query.Del("flywheel")
			r.URL.RawQuery = query.Encode()
			w.Header().Set("Content-Type", "application/json")
			if acceptHtml {
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
		body := fmt.Sprintf(HTML_ERROR, pong.Err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(body))
		return
	}

	switch pong.Status {
	case STOPPED:
		query.Set("flywheel", "start")
		r.URL.RawQuery = query.Encode()
		body := fmt.Sprintf(HTML_STOPPED, r.URL)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(body))
	case STARTING:
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(HTML_STARTING))
	case STARTED:
		fw.Proxy(w, r)
	case STOPPING:
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(HTML_STOPPING))
	case UNHEALTHY:
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(HTML_UNHEALTHY))
	}
}
