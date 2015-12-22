package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"time"
)

type Config struct {
	Vhosts      map[string]string `json:"vhosts"`
	Endpoint    string            `json:"endpoint"`
	Instances   []string          `json:"instances"`
	HcInterval  Duration          `json:"healthcheck-interval"`
	IdleTimeout Duration          `json:"idle-timeout"`
	AutoScaling AutoScalingConfig `json:"autoscaling"`
}

type AutoScalingConfig struct {
	Terminate map[string]int64 `json:"terminate"`
	Stop      []string         `json:"stop"`
}

type Duration time.Duration

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func ReadConfig(filename string) (*Config, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	in, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	err = json.Unmarshal(in, cfg)
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

	if c.HcInterval <= 0 {
		c.HcInterval = Duration(time.Minute)
	}

	if c.IdleTimeout <= 0 {
		c.IdleTimeout = Duration(time.Minute)
	}
	return nil
}
