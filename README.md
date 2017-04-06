# Selenoid
[![Build Status](https://travis-ci.org/aandryashin/selenoid.svg?branch=master)](https://travis-ci.org/aandryashin/selenoid)
[![Coverage](https://codecov.io/github/aandryashin/selenoid/coverage.svg)](https://codecov.io/gh/aandryashin/selenoid)
[![Go Report Card](https://goreportcard.com/badge/github.com/aandryashin/selenoid)](https://goreportcard.com/report/github.com/aandryashin/selenoid)
[![Release](https://img.shields.io/github/release/aandryashin/selenoid.svg)](https://github.com/aandryashin/selenoid/releases/latest)
[![GoDoc](https://godoc.org/github.com/aandryashin/selenoid?status.svg)](https://godoc.org/github.com/aandryashin/selenoid)

Selenoid is a powerful [Go](http://golang.org/) implementation of original [Selenium](http://github.com/SeleniumHQ/selenium) hub code. It is using Docker to launch browsers.

## Quick Start Guide
1) Install [Docker](https://docs.docker.com/engine/installation/)
2) Pull browser images, e.g.:
```
$ docker pull selenoid/firefox:latest
$ docker pull selenoid/chrome:latest
```
3) Pull Selenoid image:
```
$ docker pull aandryashin/selenoid:1.0.0
```
4) Create the following configuration file:
```
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
```
5) Run Selenoid container:
```
$ docker run -d --name selenoid -p 4444:4444 -v /etc/selenoid:/etc/selenoid:ro -v /var/run/docker.sock:/var/run/docker.sock aandryashin/selenoid:1.0.0
```
6) Access Selenoid as regular Selenium hub:
```
http://localhost:4444/wd/hub
```

## Simple UI

Selenoid has standalone UI to show current quota, queue and browser usage: https://github.com/lanwen/selenoid-ui.  

## Configuration

### Flags

The following flags are supported by ```selenoid``` command:
```
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
```

### Browsers Configuration File

Selenoid uses simple JSON configuration files of the following format (we use **#** for comments here):
```
{
    "firefox": { # Browser name
      "default": "46.0", # Default browser version
      "versions": { # A list of available browser versions
        "46.0": { # Version name
          "image": "selenoid/firefox:46.0", # Image name or driver binary command
          "port": "4444", # Port to proxy connections to, see below
          "tmpfs": {"/tmp": "size=512m"}, # Optional. Add in memory filesystem (tmpfs) to container, see below
          "path" : "/wd/hub" # Optional. Path relative to / where we request a new session, see below 
        },
        "50.0" :{
            # ...
        }
      }
    },
    "chrome": {
        # ...
    }
}
```
This file represents a mapping between a list of supported browser versions and Docker container images or driver binaries.
#### Browser Name and Version
Browser name and version are just strings that are matched against Selenium desired capabilities: browserName and version. If no version capability is present default version is used. When there is no exact version match we also try match by prefix. That means version string in JSON should start with version string from capabilities. The following request matches...
```
versionFromConfig = 46.0
versionFromCapabilities = 46 # 46.0 starts with 46
```
... and the following does not:
```
versionFromConfig = 46.0
versionFromCapabilities = 46.1 # 46.0 does not start with 46.1  
```
#### Image
Image by default is a string with container specification in Docker format (hub.example.com/project/image:tag). The following image names are valid:
```
my-internal-docker-hub.example.com/selenoid/firefox:46.0 # This comes from internal Docker hub
selenoid/firefox:46.0 # This is downloaded from hub.docker.com
```
If you wish to use a standalone binary instead of Docker container, then image field should contain command specification in square brackets:
```
"46.0": { # Version name
    "image": ["/usr/bin/mybinary", "-arg1", "foo", "-arg2", "bar", "-arg3"],
    "port": ...
    "tmpfs": ...
    "path" : ... 
},
```
Selenoid proxies connections to either Selenium server or standalone driver binary. Depending on operating system both can be packaged inside Docker container.

#### Port, Tmpfs and Path
You should use **port** field to specify the real port that Selenium server or driver will listen on. For Docker containers this is a port inside container. **tmpfs** and **path** fields are optional. You may probably know that moving browser cache to in-memory filesystem ([tmpfs](https://en.wikipedia.org/wiki/Tmpfs)) can dramatically improve its performance. Selenoid can automatically attach one or more in-memory filesystems as volumes to Docker container being run. To achieve this define one or more mount points and their respective sizes in optional **tmpfs** field:
```
"46.0": { # Version name
    "image": ...
    "port": ...
    "tmpfs": {"/tmp": "size=512m", "/var": "size=128m"},
    "path" : ... 
},
```
The last field - **path** is needed to specify relative path to the URL where a new session is created (default is **/**). 

### Timezone

When used in Docker container Selenoid will have timezone set to UTC. To set custom timezone pass TZ environment variable to Docker:
```
$ docker run -d --name selenoid -p 4444:4444 -e TZ=Europe/Moscow -v /etc/selenoid:/etc/selenoid:ro -v /var/run/docker.sock:/var/run/docker.sock aandryashin/selenoid:1.0.0
```

### Logging Configuration File
By default Docker container logs are saved to host machine hard drive. When using Selenoid for local development that's ok. But in big Selenium cluster you may want to send logs to some centralized storage like [Logstash](https://www.elastic.co/products/logstash) or [Graylog](https://www.graylog.org/). Docker provides such functionality by so-called [logging drivers](https://docs.docker.com/engine/admin/logging/overview/). Selenoid logging configuration file allows to specify which logging driver to use globally for all started Docker containers with browsers. Configuration file has the following format:
```
{
    "Type" : "<driver-type>",
    "Config" : {
      "key1" : "value1",
      "key2" : "value2"
    }
}
```
Here **<driver-type>** - is a supported Docker logging driver type like ```syslog```, ```journald``` or ```awslogs```. ```Config``` is a list of key-value pairs used to configure selected driver. For example these Docker logging parameters...
```
--log-driver=syslog --log-opt syslog-address=tcp://192.168.0.42:123 --log-opt syslog-facility=daemon
```
... are equivalent to the following Selenoid logging configuration:
```
{
    "Type" : "syslog",
    "Config" : {
      "syslog-address" : "tcp://192.168.0.42:123",
      "syslog-facility" : "daemon"
    }
}
```

### Reloading Configuration

To reload configuration without restart send SIGHUP:
```
# kill -HUP <pid> # Use only one of these commands!!
# docker kill -s HUP <container-id-or-name>
```

### Usage statistics

Selenoid calculates usage statistics that can be accessed with HTTP request:
```
$ curl http://localhost:4444/status
{
    "total": 80,
    "used": 14,
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
```
Users are extracted from basic HTTP authentication headers.

## Sending statistics to external systems

To send Selenoid statistics described in previous section you can use [Telegraf](https://github.com/influxdata/telegraf). For example to send status to [Graphite](https://github.com/graphite-project):

1) Pull latest Telegraf Docker container:
```
# docker pull telegraf:alpine
```
2) Generate configuration file:
```
# mkdir -p /etc/telegraf
# docker run --rm telegraf:alpine --input-filter httpjson --output-filter graphite config > /etc/telegraf/telegraf.conf
```
3) Edit file like the following (three dots mean some not shown lines):
```
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
```
4) Start Telegraf container:
```
# docker run --net host -d --name telegraf -v /etc/telegraf:/etc telegraf:alpine --config /etc/telegraf.conf
```
Metrics will be sent to my-graphite-host.example.com:2024. Metric names will be like the following:
```
one_min.selenoid_example_com.httpjson_selenoid.pending
one_min.selenoid_example_com.httpjson_selenoid.queued
one_min.selenoid_example_com.httpjson_selenoid.total
...
```

## Development

To build Selenoid:

1) Install [Golang](https://golang.org/doc/install)
2) Setup `$GOPATH` [properly](https://github.com/golang/go/wiki/GOPATH)
3) Install [govendor](https://github.com/kardianos/govendor): 
```
$ go get -u github.com/kardianos/govendor
```
4) Get Selenoid source:
```
$ go get -d github.com/aandryashin/selenoid
```
5) Go to project directory:
```
$ cd $GOPATH/src/github.com/aandryashin/selenoid
```
6) Checkout dependencies:
```
$ govendor sync
```
7) Build source:
```
$ go build
```
8) Run Selenoid:
```
$ ./selenoid --help
```
9) To build Docker container type:
```
$ GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build
$ docker build -t selenoid:latest .
```