package flywheel

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// How often flywheel will update its internal state and/or check for idle
// timeouts
const SpinINTERVAL = time.Second

// Ping - HTTP requests "ping" the flywheel goroutine. This updates the idle timeout,
// and returns the current status to the http request.
type Ping struct {
	replyTo      chan Pong
	setTimeout   time.Duration
	requestStart bool
	requestStop  bool
	noop         bool
}

// Pong - result of the ping request
type Pong struct {
	Status      Status    `json:"-"`
	StatusName  string    `json:"status"`
	Err         error     `json:"error,omitempty"`
	LastStarted time.Time `json:"last-started,omitempty"`
	LastStopped time.Time `json:"last-stopped,omitempty"`
	StopAt      time.Time `json:"stop-due-at"`
}

// Flywheel struct holds all the state required by the flywheel goroutine.
type Flywheel struct {
	config      *Config
	running     bool
	pings       chan Ping
	status      Status
	ready       bool
	stopAt      time.Time
	lastStarted time.Time
	lastStopped time.Time
	ec2         *ec2.EC2
	autoscaling *autoscaling.AutoScaling
	hcInterval  time.Duration
	idleTimeout time.Duration
}

// New - Create new Flywheel type
func New(config *Config) *Flywheel {

	awsConfig := &aws.Config{Region: &config.Region}
	sess := session.New(awsConfig)

	return &Flywheel{
		hcInterval:  time.Duration(config.HcInterval),
		idleTimeout: time.Duration(config.IdleTimeout),
		config:      config,
		pings:       make(chan Ping),
		stopAt:      time.Now(),
		ec2:         ec2.New(sess),
		autoscaling: autoscaling.New(sess),
	}
}

// ProxyEndpoint - retrieve the reverse proxy destination
func (fw *Flywheel) ProxyEndpoint(hostname string) string {
	vhost, ok := fw.config.Vhosts[hostname]
	if ok {
		return vhost
	}
	return fw.config.Endpoint
}

// Spin - Runs the main loop for the Flywheel.
func (fw *Flywheel) Spin() {
	hchan := make(chan Status, 1)

	go fw.HealthWatcher(hchan)

	ticker := time.NewTicker(SpinINTERVAL)
	for {
		select {
		case ping := <-fw.pings:
			fw.RecvPing(&ping)
		case <-ticker.C:
			fw.Poll()
		case status := <-hchan:
			if fw.status != status {
				log.Printf("Healthcheck - status is now %v", status)
				// Status may change from STARTED to UNHEALTHY to STARTED due
				// to things like AWS RequestLimitExceeded errors.
				// If there is an active timeout, keep it instead of resetting.
				if status == STARTED && fw.stopAt.Before(time.Now()) {
					fw.stopAt = time.Now().Add(fw.idleTimeout)
					log.Printf("Timer update. Stop scheduled for %v", fw.stopAt)
				}
				fw.status = status
			}
		}
	}
}

// RecvPing - process user ping requests and update state if needed
func (fw *Flywheel) RecvPing(ping *Ping) {
	var pong Pong

	ch := ping.replyTo
	defer close(ch)

	switch fw.status {
	case STOPPED:
		if ping.requestStart {
			pong.Err = fw.Start()
		}

	case STARTED:
		if ping.noop {
			// Status requests, etc. Don't update idle timer
		} else if ping.requestStop {
			pong.Err = fw.Stop()
		} else if int64(ping.setTimeout) != 0 {
			fw.stopAt = time.Now().Add(ping.setTimeout)
			log.Printf("Timer update. Stop scheduled for %v", fw.stopAt)
		} else {
			fw.stopAt = time.Now().Add(fw.idleTimeout)
			log.Printf("Timer update. Stop scheduled for %v", fw.stopAt)
		}
	}

	pong.Status = fw.status
	pong.StatusName = fw.status.String()
	pong.LastStarted = fw.lastStarted
	pong.LastStopped = fw.lastStopped
	pong.StopAt = fw.stopAt

	ch <- pong
}

// Poll - The periodic check for starting/stopping state transitions and idle
// timeouts
func (fw *Flywheel) Poll() {
	switch fw.status {
	case STARTED:
		if time.Now().After(fw.stopAt) {
			fw.Stop()
			log.Print("Idle timeout - shutting down")
			fw.status = STOPPING
		}

	case STOPPING:
		if fw.ready {
			log.Print("Shutdown complete")
			fw.status = STOPPED
		}

	case STARTING:
		if fw.ready {
			fw.status = STARTED
			fw.stopAt = time.Now().Add(fw.idleTimeout)
			log.Printf("Startup complete. Stop scheduled for %v", fw.stopAt)
		}
	}
}

// WriteStatusFile - Before we exit the application we write the current state
func (fw *Flywheel) WriteStatusFile(statusFile string) {
	var pong Pong

	fd, err := os.OpenFile(statusFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Unable to write status file: %s", err)
		return
	}
	defer fd.Close()

	pong.Status = fw.status
	pong.StatusName = fw.status.String()
	pong.LastStarted = fw.lastStarted
	pong.LastStopped = fw.lastStopped

	buf, err := json.Marshal(pong)
	if err != nil {
		log.Printf("Unable to write status file: %s", err)
		return
	}

	_, err = fd.Write(buf)
	if err != nil {
		log.Printf("Unable to write status file: %s", err)
		return
	}
}

// ReadStatusFile load status from the status file
func (fw *Flywheel) ReadStatusFile(statusFile string) {
	fd, err := os.Open(statusFile)
	if err != nil {
		if err != os.ErrNotExist {
			log.Printf("Unable to load status file: %v", err)
		}
		return
	}

	stat, err := fd.Stat()
	if err != nil {
		log.Printf("Unable to load status file: %v", err)
		return
	}

	buf := make([]byte, int(stat.Size()))
	_, err = io.ReadFull(fd, buf)
	if err != nil {
		log.Printf("Unable to load status file: %v", err)
		return
	}

	var status Pong
	err = json.Unmarshal(buf, &status)
	if err != nil {
		log.Printf("Unable to load status file: %v", err)
		return
	}

	fw.status = status.Status
	fw.lastStarted = status.LastStarted
	fw.lastStopped = status.LastStopped
}
