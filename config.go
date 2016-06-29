package main

import (
	"encoding/json"
	"fmt"
	"io"
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

	cfg := &Config{}
	if err = cfg.Parse(fd); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Parse(rd io.Reader) error {
	in, err := ioutil.ReadAll(rd)
	if err != nil {
		return err
	}
	err = json.Unmarshal(in, c)
	if err != nil {
		return fmt.Errorf("Could not decode json: %v", err)
	}

	err = c.Validate()
	if err != nil {
		return fmt.Errorf("Invalid configuration: %v", err)
	}
	return nil
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
		return fmt.Errorf("No instances or asg configured")
	}

	if len(c.Endpoint) == 0 {
		return fmt.Errorf("No endpoint configured")
	}

	if c.HcInterval <= 0 {
		c.HcInterval = Duration(30 * time.Second)
	}

	if c.IdleTimeout <= 0 {
		c.IdleTimeout = Duration(3 * time.Hour)
	}
	return nil
}
