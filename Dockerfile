FROM alpine:3.5
MAINTAINER Ivan Krutov <vania-pooh@vania-pooh.com>

COPY selenoid /usr/bin

EXPOSE 4444
ENTRYPOINT ["/usr/bin/selenoid", "-listen", ":4444", "-conf", "/etc/selenoid/browsers.json"]
