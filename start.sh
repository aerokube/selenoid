#!/bin/bash

killall selenoid
go build
docker kill `docker ps -q`
minimesos up
./selenoid -mesos http://localhost:5050