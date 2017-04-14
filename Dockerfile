FROM alpine:3.5

RUN apk add -U tzdata && rm -Rf /var/cache/apk/*
COPY selenoid /usr/bin

EXPOSE 4444
ENTRYPOINT ["/usr/bin/selenoid", "-listen", ":4444", "-conf", "/etc/selenoid/browsers.json"]
