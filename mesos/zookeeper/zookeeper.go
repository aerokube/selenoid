package zookeeper

import (
	"strings"
	"time"
	"github.com/samuel/go-zookeeper/zk"
	"sort"
	"encoding/json"
	"strconv"
	"net/url"
	"log"
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
		log.Printf("******* create: %+v %+v\n", path, aa)
	} else {
		exists, _, _ := conn.Exists(selenoidPath + "/tasks")
		if !exists {
			flags := int32(0)
			acl := zk.WorldACL(zk.PermAll)
			aa, _ := conn.Create("/selenoid/tasks", []byte{}, flags, acl)
			log.Printf("******* create: %+v %+v\n", aa)
		} else {
			DelAllChildrenNodes()
		}
	}
}

func DetectMaster(flagUrl *url.URL) string {
	conn := connectToMesosZk(flagUrl.Host)
	defer conn.Close()
	path := flagUrl.Path
	if path == "" {
		log.Fatal("There is no path to mesos in zookeeper")
	}
	c, _, _ := conn.Children(flagUrl.Path)
	sort.Strings(c)
	data, _, _ := conn.Get(flagUrl.Path + "/" + c[0])
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
	log.Printf("******* create: %+v\n", path)
}

func CreateFrameworkNode(framewordId string) {
	DelAllFrameworkNodes()

	conn := connect()
	defer conn.Close()

	flags := int32(0)
	acl := zk.WorldACL(zk.PermAll)

	exists, _, _ := conn.Exists(selenoidPath + "/frameworkInfo")
	if !exists {
		fi, _ := conn.Create("/selenoid/frameworkInfo", []byte{}, flags, acl)
		log.Printf("******* create: %+v %+v\n", fi)
	}

	path, err := conn.Create(selenoidPath+"/frameworkInfo/"+framewordId, []byte{}, flags, acl)
	must(err)
	log.Printf("******* create FrameworkId: %+v\n", path)
}

func GetAgentIdForTask(taskId string) string{
	conn := connect()
	defer conn.Close()

	data, stat, err := conn.Get(selenoidPath + "/tasks/" + taskId)
	must(err)
	log.Printf("******* get:    %+v %+v\n", string(data), stat)
	return string(data)
}

func GetFrameworkInfo() []string{
	conn := connect()
	defer conn.Close()
	exists, _, err := conn.Exists(selenoidPath + "/frameworkInfo")
	must(err)
	if exists {
		childs, stat, err := conn.Children(selenoidPath + "/frameworkInfo")
		if err != nil {
			log.Printf("Children returned error: %+v", err)
			return nil
		}
		log.Printf("******* get FrameworkId:    %+v %+v\n", []string(childs), stat)
		return childs
	}
	return nil
}

func GetChildren() []string {
	conn := connect()
	defer conn.Close()
	exists, _, err := conn.Exists(selenoidPath + "/tasks")
	must(err)
	if exists {
		childs, stat, err := conn.Children(selenoidPath + "/tasks")
		if err != nil {
			log.Printf("Children returned error: %+v", err)
			return nil
		}
		log.Printf("******* get:    %+v %+v\n", []string(childs), stat)
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

func DelAllFrameworkNodes() {
	conn := connect()
	defer conn.Close()
	childs := GetFrameworkInfo()
	if childs != nil {
		for _, n := range childs {
			DelFrameworkNode(n)
		}
	}
}

func DelNode(taskId string) {
	conn := connect()
	defer conn.Close()

	err := conn.Delete(selenoidPath+"/tasks/"+taskId, -1)
	must(err)
	log.Printf("******* delete FrameworkId " + taskId + ": ok\n")
}

func DelFrameworkNode(id string) {
	conn := connect()
	defer conn.Close()

	err := conn.Delete(selenoidPath+"/frameworkInfo/"+id, -1)
	must(err)
	log.Printf("******* delete " + id + ": ok\n")
}

func Del() {
	conn := connect()
	defer conn.Close()

	err := conn.Delete(selenoidPath, -1)
	must(err)
	log.Printf("******* delete /Tasks: ok\n")
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

func connectToMesosZk(url string) *zk.Conn {
	zks := strings.Split(url, ",")
	conn, _, err := zk.Connect(zks, time.Minute)
	must(err)
	return conn
}
