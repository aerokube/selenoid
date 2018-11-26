FROM alpine:3.8

RUN apk add -U ca-certificates tzdata mailcap && rm -Rf /var/cache/apk/*
COPY selenoid /usr/bin

EXPOSE 4444
ENTRYPOINT ["/usr/bin/selenoid", "-listen", ":4444", "-conf", "/etc/selenoid/browsers.json", "-video-output-dir", "/opt/selenoid/video/"]
