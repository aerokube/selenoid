# Selenoid
[![Build Status](https://travis-ci.org/aerokube/selenoid.svg?branch=master)](https://travis-ci.org/aerokube/selenoid)
[![Coverage](https://codecov.io/github/aerokube/selenoid/coverage.svg)](https://codecov.io/gh/aerokube/selenoid)
[![Go Report Card](https://goreportcard.com/badge/github.com/aerokube/selenoid)](https://goreportcard.com/report/github.com/aerokube/selenoid)
[![Release](https://img.shields.io/github/release/aerokube/selenoid.svg)](https://github.com/aerokube/selenoid/releases/latest)
[![Docker Pulls](https://img.shields.io/docker/pulls/aerokube/selenoid.svg)](https://hub.docker.com/r/aerokube/selenoid)

Selenoid is a powerful [Go](http://golang.org/) implementation of original [Selenium](http://github.com/SeleniumHQ/selenium) hub code.
It is using Docker to launch browsers.

## Quick Start Guide
1) Download browser images, generate configuration file and start Selenoid:
```
$ docker run --rm                                   \
    -v /var/run/docker.sock:/var/run/docker.sock    \
    -v ${HOME}:/root                                \
    -e OVERRIDE_HOME=${HOME}                        \
    aerokube/cm:latest-release selenoid start       \
    --vnc --tmpfs 128
```
2) Access Selenoid as regular Selenium hub (works only for POST requests):
```
http://localhost:4444/wd/hub
```
More details can be found in [documentation](http://aerokube.com/selenoid/latest/).

## Simple UI

Selenoid has [standalone UI](https://github.com/aerokube/selenoid-ui) to show current quota, queue and browser usage (and much more!).

## Complete Guide & Build Instructions

Complete reference guide (including building instructions) can be found at: http://aerokube.com/selenoid/latest/
