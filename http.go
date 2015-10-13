package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

func (fw *Flywheel) SendPing(start bool) int {
	replyTo := make(chan int, 1)
	sreq := Ping{replyTo: replyTo, requestStart: start}

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
	flywheel, ok := query["flywheel"]
	status := fw.SendPing(ok && flywheel[0] == "start")

	if ok {
		query.Del("flywheel")
		r.URL.RawQuery = query.Encode()
		w.Header().Set("Location", r.URL.String())
		w.WriteHeader(302)
	}

	switch status {
	case STOPPED:
		query.Set("flywheel", "start")
		r.URL.RawQuery = query.Encode()
		body := fmt.Sprintf(`<html><body>Currently stopped. <a href="%s">Click here</a> to start.</body></html>`, r.URL)
		w.Write([]byte(body))
	case STARTING:
		w.Write([]byte("Starting environment, please wait\n"))
	case STARTED:
		fw.Proxy(w, r)
	case STOPPING:
		w.Write([]byte("Shutdown in progress...\n"))
	}
}
