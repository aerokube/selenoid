package state

import (
	"testing"

	. "github.com/aandryashin/matchers"
	"github.com/aandryashin/selenoid/docker"
)

func TestNewState(t *testing.T) {
	config, _ := docker.NewConfig("../browsers.json")
	state := NewState(config, 1)
	AssertThat(t, len(state.status.Browsers["firefox"]["48.0"]), EqualTo{0})
	AssertThat(t, len(state.status.Browsers["chrome"]["53.0"]), EqualTo{0})
}

func TestNewSession(t *testing.T) {
	config, _ := docker.NewConfig("../browsers.json")
	state := NewState(config, 1)

	state.NewSession("1", "quota", "firefox", "")
	AssertThat(t, state.status.Browsers["firefox"]["48.0"]["quota"], EqualTo{1})

	state.NewSession("2", "quota", "firefox", "48")
	AssertThat(t, state.status.Browsers["firefox"]["48.0"]["quota"], EqualTo{2})

	state.NewSession("3", "quota", "firefox", "48.0")
	AssertThat(t, state.status.Browsers["firefox"]["48.0"]["quota"], EqualTo{3})
}

func TestDeleteSession(t *testing.T) {
	config, _ := docker.NewConfig("../browsers.json")
	state := NewState(config, 1)

	state.NewSession("1", "quota", "firefox", "")
	AssertThat(t, state.status.Browsers["firefox"]["48.0"]["quota"], EqualTo{1})

	state.DeleteSession("2")
	AssertThat(t, state.status.Browsers["firefox"]["48.0"]["quota"], EqualTo{1})

	state.DeleteSession("1")
	AssertThat(t, state.status.Browsers["firefox"]["48.0"]["quota"], EqualTo{0})
}
