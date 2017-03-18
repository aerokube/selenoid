/*
Selenoid is a powerful implementation of Selenium Hub using Docker or standalone web driver binaries to start and launch browsers.

Usage

1) Install Docker (https://docs.docker.com/engine/installation/)

2) Pull browser images:
  $ docker pull selenoid/firefox:latest
  $ docker pull selenoid/chrome:latest
3) Pull Selenoid image:
  $ docker pull aandryashin/selenoid:1.0.0
4) Create the following configuration file:
  $ cat /etc/selenoid/browsers.json
  {
    "firefox": {
      "default": "latest",
      "versions": {
        "latest": {
          "image": "selenoid/firefox:latest",
          "port": "4444"
        }
      }
    },
    "chrome": {
      "default": "latest",
      "versions": {
        "latest": {
          "image": "selenoid/chrome:latest",
          "port": "4444"
        }
      }
    }
  }
5) Run Selenoid container:
  $ docker run -d --name selenoid -p 4444:4444 -v /etc/selenoid:/etc/selenoid:ro -v /var/run/docker.sock:/var/run/docker.sock aandryashin/selenoid:1.0.0
6) Access Selenoid as regular Selenium hub:
  http://localhost:4444/wd/hub

Graceful Restart

To gracefully restart (without losing connections) send SIGUSR2:
  # kill -USR2 <pid>
  # docker kill -s USR2 <container-id-or-name>

Flags

The following flags are supported:
  -conf string
    	Browsers configuration file (default "config/browsers.json")
  -disable-docker
    	Disable docker support
  -limit int
    	Simultaneous container runs (default 5)
  -listen string
    	Network address to accept connections (default ":4444")
  -log-conf string
    	Container logging configuration file (default "config/container-logs.json")
  -timeout duration
    	Session idle timeout in time.Duration format (default 1m0s)

Status

Selenoid calculates usage statistics that can be accessed with HTTP request:
  $ curl http://localhost:4444/status
  {
    "total": 80,
    "used": 0,
    "queued": 0,
    "pending": 1,
    "browsers": {
      "firefox": {
        "46.0": {
          "user1": 5,
          "user2": 6
        },
        "48.0": {
          "user2": 3
        }
      }
    }
  }
Users are extracted from basic HTTP authentication headers.

Sending status to external systems

To send Selenoid status data described above you can use Telegraf (https://github.com/influxdata/telegraf). The following steps show how to send status to Graphite:
1) Pull latest Telegraf Docker container:
  # docker pull telegraf:alpine
2) Generate configuration file:
  # mkdir -p /etc/telegraf
  # docker run --rm telegraf:alpine --input-filter httpjson --output-filter graphite config > /etc/telegraf/telegraf.conf
3) Edit file like the following (three dots mean some not shown lines):
    ...
  [agent]
  interval = "10s" # <- adjust this if needed
    ...
  [[outputs.graphite]]
    ...
  servers = ["my-graphite-host.example.com:2024"] # <- adjust host and port
    ...
  prefix = "one_min" # <- something before hostname, can be blank
    ...
  template = "host.measurement.field"
    ...
  [[inputs.httpjson]]
    ...
  name = "selenoid"
    ...
  servers = [
    "http://localhost:4444/status" # <- this is localhost only if Telegraf is run on Selenoid host
  ]
4) Start Telegraf container:
  # docker run --net host -d --name telegraf -v /etc/telegraf:/etc telegraf:alpine --config /etc/telegraf.conf
Metrics will be sent to my-graphite-host.example.com:2024. Metric names will be like the following:
  one_min.selenoid_example_com.httpjson_selenoid.pending
  one_min.selenoid_example_com.httpjson_selenoid.queued
  one_min.selenoid_example_com.httpjson_selenoid.total
  ...

Building

See https://github.com/aandryashin/selenoid.
*/
package main
