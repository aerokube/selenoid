package zookeeper

import (
	"fmt"
	"strings"
	"time"
	"github.com/samuel/go-zookeeper/zk"
	"sort"
	"encoding/json"
	"strconv"
)

const (
	selenoidPath = "/selenoid"
)

var Zk *Zoo

type Zoo struct {
	Url string
}

type MesosConfig struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
}

func Create() {
	conn := connect()
	defer conn.Close()
	exists, _, _ := conn.Exists(selenoidPath)
	if !exists {
		flags := int32(0)
		acl := zk.WorldACL(zk.PermAll)
		path, err := conn.Create(selenoidPath, []byte{}, flags, acl)
		must(err)

		aa, _ := conn.Create("/selenoid/tasks", []byte{}, flags, acl)
		fmt.Printf("******* create: %+v %+v\n", path, aa)
	} else {
		exists, _, _ := conn.Exists(selenoidPath + "/tasks")
		if !exists {
			flags := int32(0)
			acl := zk.WorldACL(zk.PermAll)
			aa, _ := conn.Create("/selenoid/tasks", []byte{}, flags, acl)
			fmt.Printf("******* create: %+v %+v\n", aa)
		} else {
			DelAllChildrenNodes()
		}
	}
}

func DetectMaster() string {
	conn := connect()
	defer conn.Close()
	c, _, _ := conn.Children("/mesos")
	sort.Strings(c)
	data, _, _ := conn.Get("/mesos/" + c[0])
	var config MesosConfig
	json.Unmarshal(data, &config)
	return "http://" + config.Hostname + ":" + strconv.Itoa(config.Port)
}

func CreateTaskNode(taskId string, agentId string) {
	conn := connect()
	defer conn.Close()

	flags := int32(0)
	acl := zk.WorldACL(zk.PermAll)

	path, err := conn.Create(selenoidPath+"/tasks/"+taskId, []byte(agentId), flags, acl)
	must(err)
	fmt.Printf("******* create: %+v\n", path)
}

func GetAgentIdForTask(taskId string) string{
	conn := connect()
	defer conn.Close()

	data, stat, err := conn.Get(selenoidPath + "/tasks/" + taskId)
	must(err)
	fmt.Printf("******* get:    %+v %+v\n", string(data), stat)
	return string(data)
}

func GetChildren() []string {
	conn := connect()
	defer conn.Close()
	exists, _, err := conn.Exists(selenoidPath + "/tasks")
	must(err)
	if exists {
		childs, stat, err := conn.Children(selenoidPath + "/tasks")
		if err != nil {
			fmt.Printf("Children returned error: %+v", err)
			return nil
		}
		fmt.Printf("******* get:    %+v %+v\n", []string(childs), stat)
		return childs
	}
	return nil
}

func DelAllChildrenNodes() {
	conn := connect()
	defer conn.Close()
	childs := GetChildren()
	if childs != nil {
		for _, n := range childs {
			DelNode(n)
		}
	}
}

func DelNode(taskId string) {
	conn := connect()
	defer conn.Close()

	err := conn.Delete(selenoidPath+"/tasks/"+taskId, -1)
	must(err)
	fmt.Printf("******* delete" + taskId + ": ok\n")
}

func Del() {
	conn := connect()
	defer conn.Close()

	err := conn.Delete(selenoidPath, -1)
	must(err)
	fmt.Printf("******* delete /Tasks: ok\n")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func connect() *zk.Conn {
	zks := strings.Split(Zk.Url, ",")
	conn, _, err := zk.Connect(zks, time.Minute)
	must(err)
	return conn
}
