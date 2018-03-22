# Selenoid
[![Build Status](https://travis-ci.org/aerokube/selenoid.svg?branch=master)](https://travis-ci.org/aerokube/selenoid)
[![Coverage](https://codecov.io/github/aerokube/selenoid/coverage.svg)](https://codecov.io/gh/aerokube/selenoid)
[![Go Report Card](https://goreportcard.com/badge/github.com/aerokube/selenoid)](https://goreportcard.com/report/github.com/aerokube/selenoid)
[![Release](https://img.shields.io/github/release/aerokube/selenoid.svg)](https://github.com/aerokube/selenoid/releases/latest)
[![Docker Pulls](https://img.shields.io/docker/pulls/aerokube/selenoid.svg)](https://hub.docker.com/r/aerokube/selenoid)
[![StackOverflow Tag](https://img.shields.io/badge/stackoverflow-selenoid-orange.svg?style=flat)](https://stackoverflow.com/questions/tagged/selenoid)

Selenoid is a powerful implementation of [Selenium](http://github.com/SeleniumHQ/selenium) hub using [Docker](https://docker.com/) containers to launch browsers.
![Selenoid Animation](docs/img/selenoid-animation.gif)

## Features

### One-command Installation
Start browser automation in minutes by copy-pasting just **one command**:
```
$ docker run --rm                                   \
    -v /var/run/docker.sock:/var/run/docker.sock    \
    -v ${HOME}:/root                                \
    -e OVERRIDE_HOME=${HOME}                        \
    aerokube/cm:latest-release selenoid start       \
    --vnc --tmpfs 128
```
**That's it!** You can now use Selenoid instead of Selenium server. Specify the following Selenium URL in tests:
```
http://localhost:4444/wd/hub
```

### Ready to use Browser Images
No need to manually install browsers or dive into WebDriver documentation. Available images:
![Browsers List](docs/img/browsers-list.gif)

New images are added right after official releases. You can create your custom images with browsers. 

### Live Browser Screen and Logs
New **[rich user interface]((https://github.com/aerokube/selenoid-ui))** showing browser screen and Selenium session logs:
![Selenoid UI](docs/img/selenoid-ui.png)

### Lightweight and Lightning Fast
Suitable for personal usage and in big clusters:
* Consumes **10 times** less memory than Java-based Selenium server under the same load
* **Small 6 Mb binary** with no external dependencies (no need to install Java)
* **Browser consumption API** working out of the box
* Ability to send browser logs to **centralized log storage** (e.g. to the [ELK-stack](https://logz.io/learn/complete-guide-elk-stack/))
* Fully **isolated** and **reproducible** environment

### Detailed Documentation and Free Support
Maintained by a growing community:
* Detailed [documentation](http://aerokube.com/selenoid/latest/)
* Telegram [support channel](https://t.me/aerokube)
* Support by [email](mailto:support@aerokube.com)
* StackOverflow [tag](https://stackoverflow.com/questions/tagged/selenoid)

## Complete Guide & Build Instructions

Complete reference guide (including building instructions) can be found at: http://aerokube.com/selenoid/latest/

## Known Users

[![JetBrains](docs/img/logo/jetbrains.png)](http://jetbrains.com/) [![Yandex](docs/img/logo/yandex.png)](https://yandex.com/company/) [![Sberbank Technology](docs/img/logo/sbertech.png)](http://sber-tech.com/) [![PropellerAds](docs/img/logo/propellerads.png)](http://propellerads.com/)

