package service

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os/exec"
	"syscall"
	"time"

	"github.com/aandryashin/selenoid/config"
)

type Driver struct {
	Service *config.Browser
}

func (d *Driver) StartWithCancel() (*url.URL, func(), error) {
	slice, ok := d.Service.Image.([]interface{})
	if !ok {
		e := fmt.Errorf("configuration error: image is not an array: %v", d.Service.Image)
		log.Println(e)
		return nil, nil, e
	}
	cmdLine := []string{}
	for _, c := range slice {
		if _, ok := c.(string); !ok {
			e := fmt.Errorf("configuration error: value is not a string: %v", c)
			log.Println(e)
			return nil, nil, e
		}
		cmdLine = append(cmdLine, c.(string))
	}
	if len(cmdLine) == 0 {
		e := fmt.Errorf("configuration error: image is empty")
		log.Println(e)
		return nil, nil, e
	}
	log.Println("Trying to allocate port")
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		e := fmt.Errorf("cannot bind to port: %v", err)
		log.Println(e)
		return nil, nil, e
	}
	u := &url.URL{Scheme: "http", Host: l.Addr().String()}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	log.Println("Available port is:", port)
	cmdLine = append(cmdLine, fmt.Sprintf("--port=%s", port))
	cmd := exec.Command(cmdLine[0], cmdLine[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	l.Close()
	log.Println("Starting process:", cmdLine)
	s := time.Now()
	err = cmd.Start()
	if err != nil {
		e := fmt.Errorf("cannot start process %v: %v", cmdLine, err)
		log.Println(e)
		return nil, nil, e
	}
	err = wait(u.String(), 10*time.Second)
	if err != nil {
		log.Println(err)
		return nil, nil, err
	}
	log.Printf("Process %d started in: %v\n", cmd.Process.Pid, time.Since(s))
	log.Println("Proxying requests to:", u.String())
	return u, func() {
		log.Println("Terminating process", cmd.Process.Pid)
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			log.Println("cannot get process group id: %v", err)
			return
		}
		err = syscall.Kill(-pgid, syscall.SIGTERM)
		if err != nil {
			log.Println("cannot terminate process %d: %v", cmd.Process.Pid, err)
			return
		}
		log.Printf("Process %d terminated\n", cmd.Process.Pid)
	}, nil
}
