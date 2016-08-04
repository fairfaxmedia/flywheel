package flywheel

import (
	"bytes"
	"testing"
	"time"
)

var configJSONV0_1 = `
{
  "idle-timeout": "3h",
  "aws_region" : "us-west-2",
  "healthcheck-interval": "30s",
  "endpoint": "dev.example.com",
  "vhosts": {
    "alt-site.example.com": "dev2.example.com"
  },
  "instances": [
    "i-deadbeef",
    "i-cafebabe"
  ],
  "autoscaling": {
    "terminate": {
      "my-safe-scaling-group": 2
    },
    "stop": [
      "my-unsafe-scaling-group"
    ]
  }
}
`

func TestDefaultConfig(t *testing.T) {
	c := &Config{}

	if err := c.Parse(bytes.NewBufferString(configJSONV0_1)); err != nil {
		t.Errorf("Expexted no error, but got %s", err)
	}

	if c.IdleTimeout != Duration(3*time.Hour) {
		t.Errorf("Expexted idle-timeout 3h, but got %v", c.IdleTimeout)
	}

	if c.HcInterval != Duration(30*time.Second) {
		t.Errorf("Expexted idle-timeout 30s, but got %v", c.HcInterval)
	}

	if c.Region != "us-west-2" {
		t.Errorf("Expexted region 'us-west-2', but got %v", c.Region)
	}

}

// Missing instances and autoscaling groups
var configBrokenJSONV0_1 = `
{
  "endpoint": "dev.example.com",
  "vhosts": {
    "alt-site.example.com": "dev2.example.com"
  }
}
`

func TestMissingInsAsgConfig(t *testing.T) {
	c := &Config{}

	if err := c.Parse(bytes.NewBufferString(configBrokenJSONV0_1)); err == nil {
		t.Errorf("Expexted an error, but got %s", err)
	}

}

// Missing instances and autoscaling groups
var configDefaultJSONV0_1 = `
{
  "endpoint": "dev.example.com",
  "vhosts": {
    "alt-site.example.com": "dev2.example.com"
  },
  "instances": [
    "i-deadbeef"
  ]
}
`

func TestDefaultValuesConfig(t *testing.T) {
	c := &Config{}

	if err := c.Parse(bytes.NewBufferString(configDefaultJSONV0_1)); err != nil {
		t.Errorf("Expexted no error, but got %s", err)
	}

	if c.Instances[0] != "i-deadbeef" {
		t.Errorf("Expected instance i-deadbeef, but got %s", c.Instances[0])
	}
	if c.IdleTimeout != Duration(3*time.Hour) {
		t.Errorf("Expexted idle-timeout 3h, but got %v", c.IdleTimeout)
	}

	if c.HcInterval != Duration(30*time.Second) {
		t.Errorf("Expexted idle-timeout 30s, but got %v", c.HcInterval)
	}

}

// Missing instances and autoscaling groups
var configMissingEndPointJSONV0_1 = `
{
  "vhosts": {
    "alt-site.example.com": "dev2.example.com"
  },
  "instances": [
    "i-deadbeef"
  ]
}
`

func TestMissingEndpointConfig(t *testing.T) {
	c := &Config{}

	if err := c.Parse(bytes.NewBufferString(configMissingEndPointJSONV0_1)); err == nil {
		t.Errorf("Expexted no error, but got %s", err)
	}

}
