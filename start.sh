#!/usr/bin/env bash


killall selenoid
go build
docker kill `docker ps -q`
docker-compose up -d
./selenoid -mesos http://localhost:5050
