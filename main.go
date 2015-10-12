package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	var err error
	var config *Config

	var listen string
	var configFile string

	flag.StringVar(&listen, "listen", "0.0.0.0:80", "Address and port to listen on")
	flag.StringVar(&configFile, "config", "", "Config file to read settings from")
	flag.Parse()

	if configFile != "" {
		config, err = ReadConfig(configFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	flywheel := New(config)

	go flywheel.Spin()

	http.Handle("/", flywheel)

	log.Print("Flywheel starting")
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		log.Fatal(err)
	}
}
