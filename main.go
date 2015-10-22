package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
)

func readStatusFile(statusFile string) *Pong {
	fd, err := os.Open(statusFile)
	if err != nil {
		if err != os.ErrNotExist {
			log.Printf("Unable to load status file: %v", err)
		}
		return nil
	}

	stat, err := fd.Stat()
	if err != nil {
		log.Printf("Unable to load status file: %v", err)
		return nil
	}

	buf := make([]byte, int(stat.Size()))
	_, err = io.ReadFull(fd, buf)
	if err != nil {
		log.Printf("Unable to load status file: %v", err)
		return nil
	}

	var status Pong
	err = json.Unmarshal(buf, &status)
	if err != nil {
		log.Printf("Unable to load status file: %v", err)
		return nil
	}

	return &status
}

func writeStatusFile(statusFile string, flywheel *Flywheel) {
	var pong Pong

	fd, err := os.OpenFile(statusFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Print("Unable to write status file: %v", err)
		return
	}
	defer fd.Close()

	pong.Status = flywheel.status
	pong.StatusName = StatusString(flywheel.status)
	pong.LastStarted = flywheel.lastStarted
	pong.LastStopped = flywheel.lastStopped

	buf, err := json.Marshal(pong)
	if err != nil {
		log.Print("Unable to write status file: %v", err)
		return
	}

	_, err = fd.Write(buf)
	if err != nil {
		log.Print("Unable to write status file: %v", err)
		return
	}
}

func main() {
	var err error
	var config *Config

	var listen string
	var configFile string
	var statusFile string

	flag.StringVar(&listen, "listen", "0.0.0.0:80", "Address and port to listen on")
	flag.StringVar(&configFile, "config", "", "Config file to read settings from")
	flag.StringVar(&statusFile, "status-file", "", "File to save runtime status to")
	flag.Parse()

	if configFile != "" {
		config, err = ReadConfig(configFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	flywheel := New(config)

	if statusFile != "" {
		status := readStatusFile(statusFile)
		if status != nil {
			flywheel.status = status.Status
			flywheel.lastStarted = status.LastStarted
			flywheel.lastStopped = status.LastStopped
		}
		defer writeStatusFile(statusFile, flywheel)
	}

	go flywheel.Spin()

	http.Handle("/", flywheel)

	log.Print("Flywheel starting")
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		log.Fatal(err)
	}
}
