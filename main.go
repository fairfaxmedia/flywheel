package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"
	"time"
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
	var setuid string

	flag.StringVar(&listen, "listen", "0.0.0.0:80", "Address and port to listen on")
	flag.StringVar(&configFile, "config", "", "Config file to read settings from")
	flag.StringVar(&statusFile, "status-file", "", "File to save runtime status to")
	flag.StringVar(&setuid, "setuid", "", "Switch to user after opening socket")
	flag.Parse()

	sock, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatal(err)
	}

	if setuid != "" {
		user, err := user.Lookup(setuid)
		if err != nil {
			log.Fatal(err)
		}
		uid, err := strconv.ParseInt(user.Uid, 10, 64)
		if err != nil {
			log.Fatalf("Invalid uid (%s: %s): %v", setuid, user.Uid, err)
		}

		err = syscall.Setuid(int(uid))
		if err != nil {
			log.Fatal(err)
		}
	}

	if configFile == "" {
		fmt.Println("Config file missing. Please run with -help for more info")
		os.Exit(2)
	}

	config, err = ReadConfig(configFile)
	if err != nil {
		log.Fatal(err)
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

	handler := &Handler{
		flywheel: flywheel,
	}

	http.Handle("/", handler)

	go func() {
		log.Print("Flywheel starting")
		err = http.Serve(sock, nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	sock.Close()
	log.Print("Stopping flywheel...")
	time.Sleep(3 * time.Second)
}
