package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
)

type Config struct {
	Vhosts      map[string]string `json:"vhosts"`
	Endpoint    string            `json:"endpoint"`
	Instances   []string          `json:"instances"`
	HcInterval  string            `json:"healthcheck-interval"`
	IdleTimeout string            `json:"idle-timeout"`
	AutoScaling AutoScalingConfig `json:"autoscaling"`
}

type AutoScalingConfig struct {
	Terminate map[string]int64 `json:"terminate"`
	Stop      []string         `json:"stop"`
}

func ReadConfig(filename string) (*Config, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	buf := bytes.Buffer{}
	buf.ReadFrom(fd)

	cfg := &Config{}
	err = json.Unmarshal(buf.Bytes(), cfg)
	if err != nil {
		return nil, fmt.Errorf("Could not decode json: %v", err)
	}

	err = cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("Invalid configuration: %v", err)
	}

	return cfg, nil
}

func (c *Config) AwsInstances() []*string {
	awsIds := make([]*string, len(c.Instances))
	for i := range c.Instances {
		awsIds[i] = &c.Instances[i]
	}
	return awsIds
}

func (c *Config) EndpointURL() (*url.URL, error) {
	return url.Parse(c.Endpoint)
}

func (c *Config) Validate() error {
	if len(c.Instances) == 0 && len(c.AutoScaling.Stop) == 0 && len(c.AutoScaling.Terminate) == 0 {
		return fmt.Errorf("No instances configured")
	}

	if len(c.Endpoint) == 0 {
		return fmt.Errorf("No endpoint configured")
	}

	return nil
}
