package zookeeper

import (
	"github.com/samuel/go-zookeeper/zk"
	"fmt"
	"strings"
	"time"
)
const path = "/Tasks"

func CreateZk() {
	conn := connect("0.0.0.0:2181")
	defer conn.Close()

	flags := int32(0)
	acl := zk.WorldACL(zk.PermAll)

	path, err := conn.Create(path, []byte("data"), flags, acl)
	must(err)
	fmt.Printf("******* create: %+v\n", path)
}

func CreateChildNodeZk(taskId string) {
	conn := connect("0.0.0.0:2181")
	defer conn.Close()

	flags := int32(0)
	acl := zk.WorldACL(zk.PermAll)

	path, err := conn.Create(path + "/"+taskId, []byte("data"), flags, acl)
	must(err)
	fmt.Printf("******* create: %+v\n", path)
}

func GetNodeZk(taskId string) {
	conn := connect("0.0.0.0:2181")
	defer conn.Close()

	data, stat, err := conn.Get(path + "/" + taskId)
	must(err)
	fmt.Printf("******* get:    %+v %+v\n", string(data), stat)
}

func getChildrenZk() ([]string) {
	conn := connect("0.0.0.0:2181")
	defer conn.Close()
	exists, _, err := conn.Exists(path)
	must(err)
	if exists {
		childs, stat, err := conn.Children(path + "/")
		must(err)
		fmt.Printf("******* get:    %+v %+v\n", []string(childs), stat)
		return childs
	}
	return nil
}

func DelAllChildrenNodesZk() {
	conn := connect("0.0.0.0:2181")
	defer conn.Close()
	childs := getChildrenZk()
	if childs != nil {
		for _, n := range childs {
			DelNodeZk(n)
		}
	}
}

func DelNodeZk(taskId string) {
	conn := connect("0.0.0.0:2181")
	defer conn.Close()

	err := conn.Delete(path + "/" + taskId, -1)
	must(err)
	fmt.Printf("******* delete" + taskId + ": ok\n")
}

func DelZk() {
	conn := connect("0.0.0.0:2181")
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
