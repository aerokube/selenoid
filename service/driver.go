package service

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os/exec"
	"time"

	"errors"
	"github.com/aandryashin/selenoid/config"
)

// Driver - driver processes manager
type Driver struct {
	Service *config.Browser
}

// StartWithCancel - Starter interface implementation
func (d *Driver) StartWithCancel() (*url.URL, func(), error) {
	slice, ok := d.Service.Image.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("configuration error: image is not an array: %v", d.Service.Image)
	}
	cmdLine := []string{}
	for _, c := range slice {
		if _, ok := c.(string); !ok {
			return nil, nil, fmt.Errorf("configuration error: value is not a string: %v", c)
		}
		cmdLine = append(cmdLine, c.(string))
	}
	if len(cmdLine) == 0 {
		return nil, nil, errors.New("configuration error: image is empty")
	}
	log.Println("Trying to allocate port")
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, nil, fmt.Errorf("cannot bind to port: %v", err)
	}
	u := &url.URL{Scheme: "http", Host: l.Addr().String()}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	log.Println("Available port is:", port)
	cmdLine = append(cmdLine, fmt.Sprintf("--port=%s", port))
	cmd := exec.Command(cmdLine[0], cmdLine[1:]...)
	l.Close()
	log.Println("Starting process:", cmdLine)
	s := time.Now()
	err = cmd.Start()
	if err != nil {
		e := fmt.Errorf("Cannot start process %v: %v", cmdLine, err)
		log.Println(e)
		return nil, nil, e
	}
	err = wait(u.String(), 10*time.Second)
	if err != nil {
		stopProcess(cmd)
		return nil, nil, err
	}
	log.Printf("Process %d started in: %v\n", cmd.Process.Pid, time.Since(s))
	log.Println("Proxying requests to:", u.String())
	return u, func() { stopProcess(cmd) }, nil
}

func stopProcess(cmd *exec.Cmd) {
	log.Println("Terminating process", cmd.Process.Pid)
	err := cmd.Process.Kill()
	if err != nil {
		log.Printf("Cannot terminate process %d: %v\n", cmd.Process.Pid, err)
		return
	}
	log.Printf("Process %d terminated\n", cmd.Process.Pid)
}
