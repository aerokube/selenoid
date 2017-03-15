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
  -conf /path/to/config.json - path to main Selenoid config (default - config/browsers.json)
  -disable-docker - whether to disable Docker functionality (used to start processes instead of containers, default - false)
  -log-conf /path/to/container-logs.json - path to container logging Selenoid config (default - config/container-logs.json)
  -limit 5 - max number of simultaneous sessions to create (default - 5)
  -listen host:port - host and port to listen on (default - :4444)
  -timeout 60s - session idle timeout, written like 30s, 1m and so on (default - 60s)

Building

See https://github.com/aandryashin/selenoid.
*/
package main
