package main

import (
	"fmt"
	. "github.com/aandryashin/matchers"
	"github.com/aandryashin/selenoid/config"
	"github.com/aandryashin/selenoid/session"
	"io/ioutil"
	"log"
	"os"
	"testing"
)

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
	err := conf.Load(confFile, logConfPath)
	AssertThat(t, err, Is{nil})
}

func TestConfigError(t *testing.T) {
	confFile := configfile(`{}`)
	os.Remove(confFile)
	conf := config.NewConfig()
	err := conf.Load(confFile, logConfPath)
	log.Println(err)
	AssertThat(t, err.Error(), EqualTo{fmt.Sprintf("browsers config: read error: open %s: no such file or directory", confFile)})
}

func TestConfigParseError(t *testing.T) {
	confFile := configfile(`{`)
	defer os.Remove(confFile)
	var conf config.Config
	err := conf.Load(confFile, logConfPath)
	AssertThat(t, err.Error(), EqualTo{"browsers config: parse error: unexpected end of JSON input"})
}

func TestConfigEmptyState(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	state := conf.State(session.NewMap(), 0, 0, 0)
	fmt.Println(state)
	AssertThat(t, state.Total, EqualTo{0})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{0})
}

func TestConfigNonEmptyState(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Browser: "firefox", Version: "49.0", Quota: "unknown"})
	state := conf.State(sessions, 1, 0, 0)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"], EqualTo{1})
}

func TestConfigEmptyVersions(t *testing.T) {
	confFile := configfile(`{"firefox":{}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Browser: "firefox", Version: "49.0", Quota: "unknown"})
	state := conf.State(sessions, 1, 0, 0)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"], EqualTo{1})
}

func TestConfigNonEmptyVersions(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Browser: "firefox", Version: "49.0", Quota: "unknown"})
	state := conf.State(sessions, 1, 0, 0)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Pending, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"], EqualTo{1})
}

func TestConfigFindMissingBrowser(t *testing.T) {
	confFile := configfile(`{}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	v := ""
	_, ok := conf.Find("firefox", &v)
	AssertThat(t, ok, Is{false})
}

func TestConfigFindDefaultVersionError(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":""}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	v := ""
	_, ok := conf.Find("firefox", &v)
	AssertThat(t, ok, Is{false})
}

func TestConfigFindDefaultVersion(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0"}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	v := ""
	_, ok := conf.Find("firefox", &v)
	AssertThat(t, ok, Is{false})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByEmptyPrefix(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	v := ""
	_, ok := conf.Find("firefox", &v)
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByPrefix(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	v := "49"
	_, ok := conf.Find("firefox", &v)
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByMatch(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	v := "49.0"
	_, ok := conf.Find("firefox", &v)
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindImage(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{"image":"image","port":"5555", "path":"/"}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	conf.Load(confFile, logConfPath)

	v := "49.0"
	b, ok := conf.Find("firefox", &v)
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
		conf.Load(confFile, logConfPath)
		done <- struct{}{}
	}()
	conf.Load(confFile, logConfPath)
	<-done
}

func TestConfigConcurrentLoadAndRead(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{"image":"image","port":"5555", "path":"/", "tmpfs": {"/tmp":"size=64k"}}}}}`)
	defer os.Remove(confFile)
	conf := config.NewConfig()
	err := conf.Load(confFile, logConfPath)
	if err != nil {
		t.Error(err)
	}
	done := make(chan string)
	go func() {
		v := ""
		browser, _ := conf.Find("firefox", &v)
		done <- browser.Tmpfs["/tmp"]
	}()
	conf.Load(confFile, logConfPath)
	<-done
}

func TestConfigConcurrentRead(t *testing.T) {
	confFile := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{"image":"image","port":"5555", "path":"/", "tmpfs": {"/tmp":"size=64k"}}}}}`)
	defer os.Remove(confFile)
	var conf config.Config
	err := conf.Load(confFile, logConfPath)
	if err != nil {
		t.Error(err)
	}
	done := make(chan string)
	go func() {
		v := ""
		browser, _ := conf.Find("firefox", &v)
		done <- browser.Tmpfs["/tmp"]
	}()
	go func() {
		v := ""
		browser, _ := conf.Find("firefox", &v)
		done <- browser.Tmpfs["/tmp"]
	}()
	<-done
	<-done
}