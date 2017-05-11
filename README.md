# Selenoid
[![Build Status](https://travis-ci.org/aerokube/selenoid.svg?branch=master)](https://travis-ci.org/aerokube/selenoid)
[![Coverage](https://codecov.io/github/aerokube/selenoid/coverage.svg)](https://codecov.io/gh/aerokube/selenoid)
[![Go Report Card](https://goreportcard.com/badge/github.com/aerokube/selenoid)](https://goreportcard.com/report/github.com/aerokube/selenoid)
[![Release](https://img.shields.io/github/release/aerokube/selenoid.svg)](https://github.com/aerokube/selenoid/releases/latest)
[![GoDoc](https://godoc.org/github.com/aerokube/selenoid?status.svg)](https://godoc.org/github.com/aerokube/selenoid)

Selenoid is a powerful [Go](http://golang.org/) implementation of original [Selenium](http://github.com/SeleniumHQ/selenium) hub code.
It is using Docker to launch browsers.

## Quick Start Guide
1) Download browser images and generate configuration file (*CM* will do all the work):
```
$ docker run --rm                                   \
    -v /var/run/docker.sock:/var/run/docker.sock    \
    aerokube/cm:1.0.0 selenoid                      \
    --last-versions 2                               \
    --tmpfs 128 --pull > browsers.json
```
2) Start Selenoid:
```
# docker run -d --name selenoid                     \
    -p 4444:4444                                    \
    -v `pwd`:/etc/selenoid:ro                       \
    -v /var/run/docker.sock:/var/run/docker.sock    \
    aerokube/selenoid
```
3) Access Selenoid as regular Selenium hub (works only for POST requests):
```
http://localhost:4444/wd/hub
```

## Simple UI

Selenoid has [standalone UI](https://github.com/aerokube/selenoid-ui) to show current quota, queue and browser usage (and much more!).

## Complete Guide & Build Instructions

Complete reference guide (including how to build) can be found at: http://aerokube.com/selenoid/latest/
