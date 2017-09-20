package service

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os/exec"
	"time"

	"errors"
	"os"
)

// Driver - driver processes manager
type Driver struct {
	ServiceBase
	Environment
}

// StartWithCancel - Starter interface implementation
func (d *Driver) StartWithCancel() (*StartedService, error) {
	requestId := d.RequestId
	slice, ok := d.Service.Image.([]interface{})
	if !ok {
		return nil, fmt.Errorf("configuration error: image is not an array: %v", d.Service.Image)
	}
	cmdLine := []string{}
	for _, c := range slice {
		if _, ok := c.(string); !ok {
			return nil, fmt.Errorf("configuration error: value is not a string: %v", c)
		}
		cmdLine = append(cmdLine, c.(string))
	}
	if len(cmdLine) == 0 {
		return nil, errors.New("configuration error: image is empty")
	}
	log.Printf("[%d] [ALLOCATING_PORT]\n", requestId)
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("cannot bind to port: %v", err)
	}
	u := &url.URL{Scheme: "http", Host: l.Addr().String()}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	log.Printf("[%d] [ALLOCATED_PORT] [%s]\n", requestId, port)
	cmdLine = append(cmdLine, fmt.Sprintf("--port=%s", port))
	cmd := exec.Command(cmdLine[0], cmdLine[1:]...)
	cmd.Env = append(cmd.Env, d.Service.Env...)
	if d.CaptureDriverLogs {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	l.Close()
	log.Printf("[%d] [STARTING_PROCESS] [%s]\n", requestId, cmdLine)
	s := time.Now()
	err = cmd.Start()
	if err != nil {
		e := fmt.Errorf("cannot start process %v: %v", cmdLine, err)
		return nil, e
	}
	err = wait(u.String(), d.StartupTimeout)
	if err != nil {
		d.stopProcess(cmd)
		return nil, err
	}
	log.Printf("[%d] [PROCESS_STARTED] [%d] [%v]\n", requestId, cmd.Process.Pid, time.Since(s))
	log.Printf("[%d] [PROXYING_REQUESTS] [%s]\n", requestId, u.String())
	return &StartedService{Url: u, Cancel: func() { d.stopProcess(cmd) }}, nil
}

func (d *Driver) stopProcess(cmd *exec.Cmd) {
	log.Printf("[%d] [TERMINATING_PROCESS] [%d]\n", d.RequestId, cmd.Process.Pid)
	err := cmd.Process.Kill()
	if err != nil {
		log.Printf("[%d] [FAILED_TO_TERMINATE_PROCESS] [%d]: %v\n", d.RequestId, cmd.Process.Pid, err)
		return
	}
	log.Printf("[%d] [TERMINATED_PROCESS] [%d]\n", d.RequestId, cmd.Process.Pid)
}
