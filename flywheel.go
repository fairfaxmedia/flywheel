package main

import (
	// "bytes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"io"
	// "io/ioutil"
	"log"
	"net/http"
	"time"
)

const (
	STOPPED = iota
	STARTING
	STARTED
	STOPPING
)

type Ping struct {
	replyTo      chan int
	requestStart bool
}

type Flywheel struct {
	config         *Config
	running        bool
	statusRequests chan Ping
	status         int
	stopAt         time.Time
	ec2            *ec2.EC2
}

func New(config *Config) *Flywheel {
	region := "ap-southeast-2"
	return &Flywheel{
		config:         config,
		statusRequests: make(chan Ping),
		stopAt:         time.Now(),
		ec2:            ec2.New(&aws.Config{Region: &region}),
	}
}

func (fw *Flywheel) Spin() {
	ticker := time.NewTicker(15 * time.Second)
	for {
		select {
		case ping := <-fw.statusRequests:
			fw.HandlePing(&ping)
		case <-ticker.C:
			fw.Poll()
		}
	}
}

func (fw *Flywheel) HandlePing(sr *Ping) {
	ch := sr.replyTo
	defer close(ch)

	switch fw.status {
	case STOPPED:
		if sr.requestStart {
			fw.Start()
		}

	case STARTED:
		// TODO - Healthcheck
		fw.stopAt = time.Now().Add(15 * time.Minute)
		log.Printf("Timer update. Stop scheduled for %v", fw.stopAt)
	}

	ch <- fw.status
}

func (fw *Flywheel) Poll() {
	healthy := fw.Healthcheck()

	switch fw.status {
	case STARTED:
		if time.Now().After(fw.stopAt) {
			fw.Stop()
			log.Print("Idle timeout - shutting down")
			fw.status = STOPPING
		}

	case STOPPING:
		log.Print("Shutdown complete")
		fw.status = STOPPED

	case STARTING:
		if healthy {
			fw.status = STARTED
			fw.stopAt = time.Now().Add(15 * time.Minute)
			log.Printf("Startup complete. Stop scheduled for %v", fw.stopAt)
		}
	}
}

func (fw *Flywheel) Healthcheck() bool {
	return true
}

func (fw *Flywheel) Start() {
	log.Print("Startup beginning")

	_, err := fw.ec2.StartInstances(
		&ec2.StartInstancesInput{
			InstanceIds: fw.config.AwsInstances(),
		},
	)
	if err != nil {
		log.Printf("Error starting: %v", err)
	} else {
		fw.status = STARTING
	}
}

func (fw *Flywheel) Stop() {
	log.Print("Shutdown beginning")

	_, err := fw.ec2.StopInstances(
		&ec2.StopInstancesInput{
			InstanceIds: fw.config.AwsInstances(),
		},
	)
	if err != nil {
		log.Printf("Error stopping: %v", err)
	} else {
		fw.status = STOPPING
	}
}

func (fw *Flywheel) Ping(start bool) int {
	replyTo := make(chan int, 1)
	sreq := Ping{replyTo: replyTo, requestStart: start}

	fw.statusRequests <- sreq

	status := <-replyTo
	return status
}

func (fw *Flywheel) Proxy(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}
	r.URL.Query().Del("flywheel")

	endpoint, err := fw.config.EndpointURL()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Invalid flywheel endpoint config"))
		log.Fatal("Invalid endpoint URL")
	}

	// buf := &bytes.Buffer{}
	// _, err = io.Copy(buf, r.Body)
	// if err != nil {
	// 	log.Print(err)
	// 	return
	// }
	// if buf.Len() > 0 {
	// 	log.Printf("Body: %v", buf.String())
	// }
	// r.Body = ioutil.NopCloser(buf)

	if endpoint.Scheme == "" {
		r.URL.Scheme = "http"
	} else {
		r.URL.Scheme = endpoint.Scheme
	}

	r.URL.Host = endpoint.Host
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
	log.Printf("%s %s", r.Method, r.RequestURI)

	query := r.URL.Query()
	flywheel, ok := query["flywheel"]
	status := fw.Ping(ok && flywheel[0] == "start")

	switch status {
	case STOPPED:
		w.Write([]byte("Currently stopped. Request with ?flywheel=start to start\n"))
	case STARTING:
		w.Write([]byte("Starting environment, please wait\n"))
	case STARTED:
		fw.Proxy(w, r)
	case STOPPING:
		w.Write([]byte("Shutdown in progress...\n"))
	}
}
