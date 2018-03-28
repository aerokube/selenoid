package zookeeper

import (
	"fmt"
	"strings"
	"time"

	"github.com/samuel/go-zookeeper/zk"
)

const (
	path   = "/Tasks"
	conStr = "0.0.0.0:2181"
)

func CreateZk() {
	conn := connect(conStr)
	defer conn.Close()
	exists, _, err := conn.Exists(path)
	must(err)
	if !exists {
		flags := int32(0)
		acl := zk.WorldACL(zk.PermAll)

		path, err := conn.Create(path, []byte("data"), flags, acl)
		must(err)
		fmt.Printf("******* create: %+v\n", path)
	}
}

func CreateChildNodeZk(taskId string) {
	conn := connect(conStr)
	defer conn.Close()

	flags := int32(0)
	acl := zk.WorldACL(zk.PermAll)

	path, err := conn.Create(path+"/"+taskId, []byte("data"), flags, acl)
	must(err)
	fmt.Printf("******* create: %+v\n", path)
}

func GetNodeZk(taskId string) {
	conn := connect(conStr)
	defer conn.Close()

	data, stat, err := conn.Get(path + "/" + taskId)
	must(err)
	fmt.Printf("******* get:    %+v %+v\n", string(data), stat)
}

func GetChildrenZk() []string {
	conn := connect(conStr)
	defer conn.Close()
	exists, _, err := conn.Exists(path)
	must(err)
	if exists {
		childs, stat, err := conn.Children(path)
		if err != nil {
			fmt.Printf("Children returned error: %+v", err)
			return nil
		}
		fmt.Printf("******* get:    %+v %+v\n", []string(childs), stat)
		return childs
	}
	return nil
}

func DelAllChildrenNodesZk() {
	conn := connect(conStr)
	defer conn.Close()
	childs := GetChildrenZk()
	if childs != nil {
		for _, n := range childs {
			DelNodeZk(n)
		}
	}
}

func DelNodeZk(taskId string) {
	conn := connect(conStr)
	defer conn.Close()

	err := conn.Delete(path+"/"+taskId, -1)
	must(err)
	fmt.Printf("******* delete" + taskId + ": ok\n")
}

func DelZk() {
	conn := connect(conStr)
	defer conn.Close()

	err := conn.Delete(path, -1)
	must(err)
	fmt.Printf("******* delete /Tasks: ok\n")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func connect(zksStr string) *zk.Conn {
	zks := strings.Split(zksStr, ",")
	conn, _, err := zk.Connect(zks, time.Second)
	must(err)
	return conn
}
