# Selenoid
[![Build Status](https://travis-ci.org/aandryashin/selenoid.svg?branch=master)](https://travis-ci.org/aandryashin/selenoid)
[![Coverage](https://codecov.io/github/aandryashin/selenoid/coverage.svg)](https://codecov.io/gh/aandryashin/selenoid)
[![Release](https://img.shields.io/github/release/aandryashin/selenoid.svg)](https://github.com/aandryashin/selenoid/releases/latest)

This repository contains a simplified [Go](http://golang.org/) implementation of original [Selenium](http://github.com/SeleniumHQ/selenium) hub code.

## Building
We use [godep](https://github.com/tools/godep) for dependencies management so ensure it's installed before proceeding with next steps. To build the code:

1. Checkout this source tree: ```$ git clone https://github.com/aandryashin/selenoid.git```
2. Download dependencies: ```$ godep restore```
3. Build as usually: ```$ go build```
4. Run compiled binary: ```$GOPATH/bin/selenoid```

## Usage

Usage of ./selenoid:
  -docker-image string
    	Docker container image (required)
  -driver-path string
    	Underlying webdriver path e.g. /wd/hub
  -driver-port string
    	Underlying webdriver port (default "4444")
  -httptest.serve string
    	if non-empty, httptest.NewServer serves on this address and blocks
  -listen string
    	network address to accept connections (default ":4444")
  -log-http
    	log HTTP traffic
  -timeout duration
    	session idle timeout in time.Duration format (default 1m0s)

## Running
To run Selenoid type: ```$ selenoid -nodes http://localhost:5555,http://example.com:5556/wd/hub```. By default it listens on port 4444.
