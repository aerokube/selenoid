# Selenoid
[![Build Status](https://travis-ci.org/aandryashin/selenoid.svg?branch=master)](https://travis-ci.org/aandryashin/selenoid)
[![Coverage](https://codecov.io/github/aandryashin/selenoid/coverage.svg)](https://codecov.io/gh/aandryashin/selenoid)
[![Release](https://img.shields.io/github/release/aandryashin/selenoid.svg)](https://github.com/aandryashin/selenoid/releases/latest)

This repository contains a powerful [Go](http://golang.org/) implementation of original [Selenium](http://github.com/SeleniumHQ/selenium) hub code.

## Building
* Install [Go Lang](https://golang.org/doc/install)
* Don't forget to setup `$GOPATH` [properly](https://github.com/golang/go/wiki/GOPATH)
* Download project - `go get github.com/aandryashin/selenoid`
* Go to project directory - `cd $GOTPATH/src/github.com/aandryashin/selenoid`
* Build the project - `go build`
* Show selenoid help - `./selenoid --help`

## Running
* Install [Docker](https://docs.docker.com/engine/installation/)
* Pull docker images: 
 * `docker pull selenoid/firefox`
 * `docker pull selenoid/chrome`
* Copy selenoid binary from previous section - `cp $GOTPATH/src/github.com/aandryashin/selenoid /usr/bin/selenoid`
* Copy the following configration file to `/etc/selenoid/browsers.json`
```json
{
  "firefox": {
    "default": "latest",
    "versions": {
      "latest": {
        "image": "selenoid/firefox:latest",
        "port": "4444"
      },
    }
  },
  "chrome": {
    "default": "latest",
    "versions": {
      "latest": {
        "image": "selenoid/chrome:latest",
        "port": "4444"
      },
    }
  }
}
```
* Run selenoid - `selenoid -conf /etc/selenoid/browsers.json`

## Configuration

### Limit

You can easily configure the number of simultaneously running containers:

`selenoid -limit 10 -conf /etc/selenoid/browsers.json`

## Usage

Access to remote web driver as regular selenium hub - `http://host:4444/wd/hub`
