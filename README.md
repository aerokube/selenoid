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

## Videos

Videos are not natively implemented in selenoid, but open VNC port on each node allows you to record a video using client side resources. Here is an example:       

Make sure the ports are exposed on the host machine:

1) Selenoid must be running in as a bin (not in docker) overwise the mapped ports are 'null'.
2) Docker APIs should be exposed to the outside world. (http://www.virtuallyghetto.com/2014/07/quick-tip-how-to-enable-docker-remote-api.html)

Installing the scripts that do all the heavy lifting
The only possible solution that has been discovered is the set of scripts here (http://www.unixuser.org/~euske/python/vnc2flv/). 

(06/27/17) Installation instructions are : 
```wget https://pypi.python.org/packages/1e/8e/40c71faa24e19dab555eeb25d6c07efbc503e98b0344f0b4c3131f59947f/vnc2flv-20100207.tar.gz\#md5\=8492e46496e187b49fe5569b5639804e```

`tar zxf vnc2flv-20100207.tar.gz`

`python setup.py install --prefix=/usr/local`

We have all the puzzle pieces now. It's time to roll.

1) Start your tests
2) If you use ggr, ask it which hub your test is using
3) Ask the hub for docker container id '/status'
4) Ask the hub docker for info about container with the id
5) Parse out which port is mapped to the container port '5900' (the VNC port)
6) Now we start recording! 'flvrec.py -P <filename_for_password_file> -o <output_video_filename> <hub_host> <the_vnc_port>'
Example 'flvrec.py -P password.txt -o /tmp/selenoid_videos/gimme_love.flv 172.31.11.135 32774

The recorded video file is here '/tmp/selenoid_videos/gimme_love.flv'


## Complete Guide & Build Instructions

Complete reference guide (including building instructions) can be found at: http://aerokube.com/selenoid/latest/
