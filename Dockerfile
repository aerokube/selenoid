FROM alpine:3

RUN apk add -U ca-certificates tzdata mailcap curl && rm -Rf /var/cache/apk/*
COPY selenoid /usr/bin
HEALTHCHECK --interval=60s --timeout=3s --retries=3 CMD [ "curl", "-fs", "http://localhost:4444/ping" ]

EXPOSE 4444
ENTRYPOINT ["/usr/bin/selenoid", "-listen", ":4444", "-conf", "/etc/selenoid/browsers.json", "-video-output-dir", "/opt/selenoid/video/"]
