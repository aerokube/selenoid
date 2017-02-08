# Selenoid
[![Build Status](https://travis-ci.org/aandryashin/selenoid.svg?branch=master)](https://travis-ci.org/aandryashin/selenoid)
[![Coverage](https://codecov.io/github/aandryashin/selenoid/coverage.svg)](https://codecov.io/gh/aandryashin/selenoid)
[![Go Report Card](https://goreportcard.com/badge/github.com/aandryashin/selenoid)](https://goreportcard.com/report/github.com/aandryashin/selenoid)
[![Release](https://img.shields.io/github/release/aandryashin/selenoid.svg)](https://github.com/aandryashin/selenoid/releases/latest)
[![GoDoc](https://godoc.org/github.com/aandryashin/selenoid?status.svg)](https://godoc.org/github.com/aandryashin/selenoid)

This repository contains a powerful [Go](http://golang.org/) implementation of original [Selenium](http://github.com/SeleniumHQ/selenium) hub code.

## Building
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
* Run Selenoid:
```
$ ./selenoid --help
```
* To build Docker container type:
```
$ GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build
$ docker build -t selenoid:latest .
```

## Usage

See https://godoc.org/github.com/aandryashin/selenoid.
