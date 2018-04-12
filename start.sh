#!/usr/bin/env bash


killall selenoid
go build
docker kill `docker ps -q`
