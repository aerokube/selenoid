package config

import (
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	. "github.com/aandryashin/matchers"
	"github.com/aandryashin/selenoid/session"
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
	fn := configfile(`{}`)
	defer os.Remove(fn)
	_, err := NewConfig(fn, 1)
	AssertThat(t, err, Is{nil})
}

func TestConfigError(t *testing.T) {
	fn := configfile(`{}`)
	os.Remove(fn)
	_, err := NewConfig(fn, 1)
	AssertThat(t, strings.HasPrefix(err.Error(), "error reading configuration file"), Is{true})
}

func TestConfigParseError(t *testing.T) {
	fn := configfile(`{`)
	defer os.Remove(fn)
	_, err := NewConfig(fn, 1)
	AssertThat(t, strings.HasPrefix(err.Error(), "error parsing configuration file"), Is{true})
}

func TestConfigEmptyState(t *testing.T) {
	fn := configfile(`{}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 0)

	state := config.State(session.NewMap(), 0)
	AssertThat(t, state.Total, EqualTo{0})
	AssertThat(t, state.Queued, EqualTo{0})
	AssertThat(t, state.Used, EqualTo{0})
}

func TestConfigNonEmptyState(t *testing.T) {
	fn := configfile(`{}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Browser: "firefox", Version: "49.0", Quota: "unknown"})
	state := config.State(sessions, 1)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{1})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"], EqualTo{1})
}

func TestConfigEmptyVersions(t *testing.T) {
	fn := configfile(`{"firefox":{}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Browser: "firefox", Version: "49.0", Quota: "unknown"})
	state := config.State(sessions, 1)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{1})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"], EqualTo{1})
}

func TestConfigNonEmptyVersions(t *testing.T) {
	fn := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)

	sessions := session.NewMap()
	sessions.Put("0", &session.Session{Browser: "firefox", Version: "49.0", Quota: "unknown"})
	state := config.State(sessions, 1)
	AssertThat(t, state.Total, EqualTo{1})
	AssertThat(t, state.Queued, EqualTo{1})
	AssertThat(t, state.Used, EqualTo{1})
	AssertThat(t, state.Browsers["firefox"]["49.0"]["unknown"], EqualTo{1})
}

func TestConfigFindMissingBrowser(t *testing.T) {
	fn := configfile(`{}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)
	v := ""
	_, ok := config.Find("firefox", &v)
	AssertThat(t, ok, Is{false})
}

func TestConfigFindDefaultVersionError(t *testing.T) {
	fn := configfile(`{"firefox":{"default":""}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)
	v := ""
	_, ok := config.Find("firefox", &v)
	AssertThat(t, ok, Is{false})
}

func TestConfigFindDefaultVersion(t *testing.T) {
	fn := configfile(`{"firefox":{"default":"49.0"}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)
	v := ""
	_, ok := config.Find("firefox", &v)
	AssertThat(t, ok, Is{false})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByEmptyPrefix(t *testing.T) {
	fn := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)
	v := ""
	_, ok := config.Find("firefox", &v)
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByPrefix(t *testing.T) {
	fn := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)
	v := "49"
	_, ok := config.Find("firefox", &v)
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindFoundByMatch(t *testing.T) {
	fn := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{}}}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)
	v := "49.0"
	_, ok := config.Find("firefox", &v)
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
}

func TestConfigFindImage(t *testing.T) {
	fn := configfile(`{"firefox":{"default":"49.0","versions":{"49.0":{"image":"image","port":"5555", "path":"/"}}}}`)
	defer os.Remove(fn)
	config, _ := NewConfig(fn, 1)
	v := "49.0"
	b, ok := config.Find("firefox", &v)
	AssertThat(t, ok, Is{true})
	AssertThat(t, v, EqualTo{"49.0"})
	AssertThat(t, b.Image, EqualTo{"image"})
	AssertThat(t, b.Port, EqualTo{"5555"})
	AssertThat(t, b.Path, EqualTo{"/"})
}
