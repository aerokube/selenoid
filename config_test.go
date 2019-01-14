package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	. "github.com/aandryashin/matchers"
	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/session"
)

const testLogConf = "config/container-logs.json"

func configfile(s string) string {
	tmp, err := ioutil.TempFile("", "config")
	if err != nil {
		log.Fatal(err)
	}
	_, err = tmp.Write([]byte(s))
	if err != nil {
		log.Fatal(err)
	}
	err = tmp.Close()
	if err != nil {
		log.Fatal(err)
	}
	return tmp.Name()
}

func TestConfig(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	err := conf.Load(confFile, testLogConf)
	AssertThat(t, err, Is{nil})
}

func TestConfigError(t *testing.T) {
	confFile := configfile(`{}`)
	os.Remove(confFile)
	conf := config.NewConfig()
	err := conf.Load(confFile, testLogConf)
	AssertThat(t, err.Error(), EqualTo{fmt.Sprintf("browsers config: read error: open %s: no such file or directory", confFile)})
}

func TestLogConfigError(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	err := conf.Load(confFile, "some-missing-file")
	AssertThat(t, err, Not{nil})
}

func TestConfigParseError(t *testing.T) {
	confFile := configfile(`{`)
	defer os.Remove(confFile)
	var conf config.Config
	err := conf.Load(confFile, testLogConf)
	AssertThat(t, err.Error(), EqualTo{"browsers config: parse error: unexpected end of JSON input"})
}

func TestConfigEmptyState(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	state := conf.State(session.NewMap(), 0, 0, 0)
	AssertThat(t, state.Total, EqualTo{0})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{0})
}

func TestConfigNonEmptyState(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Caps: session.Caps{Name: "firefox", Version: "49.0"}, Quota: "unknown"})
	state := conf.State(sessions, 1, 0, 0)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"].Count, EqualTo{1})
}

func TestConfigEmptyVersions(t *testing.T) {
	confFile := configfile(`{"firefox":{}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Caps: session.Caps{Name: "firefox", Version: "49.0"}, Quota: "unknown"})
	state := conf.State(sessions, 1, 0, 0)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"].Count, EqualTo{1})
}

func TestConfigNonEmptyVersions(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Caps: session.Caps{Name: "firefox", Version: "49.0"}, Quota: "unknown"})
	state := conf.State(sessions, 1, 0, 0)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"].Count, EqualTo{1})
}

func TestConfigFindMissingBrowser(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	_, _, ok := conf.Find("firefox", "")
	AssertThat(t, ok, Is{false})
}

func TestConfigFindDefaultVersionError(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":""}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	_, _, ok := conf.Find("firefox", "")
	AssertThat(t, ok, Is{false})
}

func TestConfigFindDefaultVersion(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0"}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	_, v, ok := conf.Find("firefox", "")
	AssertThat(t, ok, Is{false})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByEmptyPrefix(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	_, v, ok := conf.Find("firefox", "")
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByPrefix(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	_, v, ok := conf.Find("firefox", "49")
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByMatch(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	_, v, ok := conf.Find("firefox", "49.0")
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindImage(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{"image":"image","port":"5555", "path":"/"}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, testLogConf)

	b, v, ok := conf.Find("firefox", "49.0")
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
	AssertThat(t, b.Image, EqualTo{"image"})
	AssertThat(t, b.Port, EqualTo{"5555"})
	AssertThat(t, b.Path, EqualTo{"/"})
}

func TestConfigConcurrentLoad(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":""}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()

	done := make(chan struct{})
	go func() {
		conf.Load(confFile, testLogConf)
		done <- struct{}{}
	}()
	conf.Load(confFile, testLogConf)
	<-done
}

func TestConfigConcurrentLoadAndRead(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{"image":"image","port":"5555", "path":"/", "tmpfs": {"/tmp":"size=64k"}}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	err := conf.Load(confFile, testLogConf)
	if err != nil {
		t.Error(err)
	}
	done := make(chan string)
	go func() {
		browser, _, _ := conf.Find("firefox", "")
		done <- browser.Tmpfs["/tmp"]
	}()
	conf.Load(confFile, testLogConf)
	<-done
}

func TestConfigConcurrentRead(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{"image":"image","port":"5555", "path":"/", "tmpfs": {"/tmp":"size=64k"}}}}}`)
	defer os.Remove(confFile)
	var conf config.Config
	err := conf.Load(confFile, testLogConf)
	if err != nil {
		t.Error(err)
	}
	done := make(chan string)
	go func() {
		browser, _, _ := conf.Find("firefox", "")
		done <- browser.Tmpfs["/tmp"]
	}()
	go func() {
		browser, _, _ := conf.Find("firefox", "")
		done <- browser.Tmpfs["/tmp"]
	}()
	<-done
	<-done
}
