package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/fairfaxmedia/flywheel"
)

func main() {
	var err error
	var config *flywheel.Config

	var listen string
	var configFile string
	var statusFile string
	var setuid string
	var showVersion bool

	flag.StringVar(&listen, "listen", "0.0.0.0:80", "Address and port to listen on")
	flag.StringVar(&configFile, "config", "", "Config file to read settings from")
	flag.StringVar(&statusFile, "status-file", "", "File to save runtime status to")
	flag.StringVar(&setuid, "setuid", "", "Switch to user after opening socket")
	flag.BoolVar(&showVersion, "version", false, "show the version and exit")
	flag.Parse()

	if showVersion {
		fmt.Fprintln(os.Stdout, os.Args[0], flywheel.GetVersion())
		return
	}

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
		log.Fatal("Config file missing. Please run with -help for more info")
	}

	config, err = flywheel.ReadConfig(configFile)
	if err != nil {
		log.Fatal(err)
	}

	fw := flywheel.New(config)

	if statusFile != "" {
		fw.ReadStatusFile(statusFile)
		defer fw.WriteStatusFile(statusFile)
	}

	go fw.Spin()

	handler := flywheel.NewHandler(fw)

	http.Handle("/", handler)

	go func() {
		log.Print("Flywheel starting")
		err = http.Serve(sock, nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
	sock.Close()
	log.Print("Stopping flywheel...")
	time.Sleep(3 * time.Second)
}
