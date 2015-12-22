# Flywheel

Flywheel is a HTTP proxy which starts and stops EC2 instances sitting behind
it.

Other solutions stop and start instances on a schedule to reduce AWS EC2 costs.
While this works well when resources are used regularly, it is less than ideal
when resources are unused for weeks or months at a time.

Flywheel will automatically stop its instances when no requests have been
received for a period of time.

Requests made while powered down will be served a "Currently powered down,
click here to start" style page.

## How to use

You will need to create a JSON configuration file for flywheel, see the
Configuration section.

Then start the server: `flywheel --config my-config.json --listen 0.0.0.0:80`

## Configuration

`idle-timeout` (string) How long after last request before powering down. Uses golang duration format, e.g. 1d2h3m

`healthcheck-interval` (string) How often to poll the AWS SDK. Used to detect stopped/started. Uses golang duration format, e.g. 1d2h3m

`endpoint` (string) The hostname and optional `:port` of the webserver to proxy to

`vhosts` (object) For environments with more than one web server. A mapping of vhost hostname to endpoint hostname

`instances` (array) An array of instance ids which will be stopped and started

`autoscaling` (object) Contains sub-settings for autoscale groups to power down.

`autoscaling`/`terminate` (object) A mapping of autoscale group name to desired size. These groups will be scaled down to 0 instances when powered down.

`autoscaling`/`stop` (array) An array of autoscale group names. These groups will have their ReplaceUnhealthy process suspended, and the instances will be stopped.

### Example:

```
{
  "idle-timeout": "3h",
  "healthcheck-interval": "30s",
  "endpoint": "dev.example.com",
  "vhosts": {
    "alt-site.example.com": "dev2.example.com"
  },
  "instances": [
    "i-deadbeef",
    "i-cafebabe",
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
```
