package service

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/session"
	"github.com/aerokube/util"
	apiv1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	selenium         int32  = 4444
	fileserver       int32  = 8080
	clipboard        int32  = 9090
	vnc              int32  = 5900
	sizeLimitDefault string = "256Mi"
)

// Kubernetes pod
type Kubernetes struct {
	ServiceBase
	Environment
	session.Caps
}

// StartWithCancel - Starter interface implementation
func (k *Kubernetes) StartWithCancel() (*StartedService, error) {

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("cluster config: %v", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("cluster config: %v", err)
	}

	requestID := k.RequestId
	image := k.Service.Image.(string)
	ns := k.Environment.NameSpace
	container := parceImageName(image)

	v1Pod := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: container,
			Namespace:    ns,
			Labels:       getLabels(k.Service, k.Caps),
		},
		Spec: apiv1.PodSpec{
			Containers: []apiv1.Container{
				{
					Name:  container,
					Image: image,
					SecurityContext: &apiv1.SecurityContext{
						Privileged: &k.Privileged,
					},
					Env: getEnvVars(k.ServiceBase, k.Caps),

					Resources: getResourses(k.ServiceBase),
					Ports:     getContainerPort(),
				},
			},
			Volumes: []apiv1.Volume{
				{
					Name: "dshm",
					VolumeSource: apiv1.VolumeSource{
						EmptyDir: &apiv1.EmptyDirVolumeSource{
							Medium:    apiv1.StorageMediumMemory,
							SizeLimit: getEmptyDirSizeLimit(k.Service),
						},
					},
				},
			},
		},
	}

	podStartTime := time.Now()
	log.Printf("[%d] [CREATING_POD] [%s] [%s]", requestID, image, ns)

	podClient, err := client.CoreV1().Pods(ns).Create(v1Pod)
	pod := podClient.GetName()
	if err != nil {
		deletePod(pod, ns, client, requestID)
		return nil, fmt.Errorf("start pod: %v", err)
	}

	if err := waitForPodToBeReady(client, podClient, ns, pod, k.StartupTimeout); err != nil {
		deletePod(pod, ns, client, requestID)
		return nil, fmt.Errorf("status pod: %v", err)
	}

	log.Printf("[%d] [POD_CREATED] [%s] [%s] [%.2fs]", requestID, pod, image, util.SecondsSince(podStartTime))

	podIP := getHostIP(pod, ns, client)
	hostPort := buildHostPort(podIP, k.Caps)

	u := &url.URL{Scheme: "http", Host: hostPort.Selenium}

	if err := wait(u.String(), k.StartupTimeout); err != nil {
		deletePod(pod, ns, client, requestID)
	}

	s := StartedService{
		Url: u,
		Pod: &session.Pod{
			ID:            string(podClient.GetUID()),
			IPAddress:     podIP,
			Name:          podClient.GetName(),
			ContainerName: container,
		},
		HostPort: session.HostPort{
			Selenium:   hostPort.Selenium,
			Fileserver: hostPort.Fileserver,
			Clipboard:  hostPort.Clipboard,
			VNC:        hostPort.VNC,
		},
		Cancel: func() {
			defer deletePod(pod, ns, client, requestID)
		},
	}
	return &s, nil
}

func deletePod(name string, ns string, client *kubernetes.Clientset, requestID uint64) {
	log.Printf("[%d] [DELETING_POD] [%s] [%s]", requestID, name, ns)
	deletePolicy := metav1.DeletePropagationForeground
	err := client.CoreV1().Pods(ns).Delete(name, &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	if err != nil {
		fmt.Errorf("delete pod: %v", err)
		return
	}
	log.Printf("[%d] [POD_DELETED] [%s] [%s]", requestID, name, ns)
}

func parceImageName(image string) (container string) {
	pref, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		container = "selenoid_browser"
	}
	container = pref.ReplaceAllString(image, "-")
	return container
}

func getHostIP(name string, ns string, client *kubernetes.Clientset) string {
	ip := ""
	pods, err := client.CoreV1().Pods(ns).List(metav1.ListOptions{})
	if err != nil {
		fmt.Errorf("pods list: %v", err)
	}
	for _, pod := range pods.Items {
		if pod.Name == name {
			ip = pod.Status.PodIP
		}
	}
	return ip
}

func buildHostPort(ip string, caps session.Caps) session.HostPort {
	fn := func(ip string, servicePort int32) string {
		port := strconv.Itoa(int(servicePort))
		return net.JoinHostPort(ip, port)
	}
	hp := session.HostPort{
		Selenium:   fn(ip, selenium),
		Fileserver: fn(ip, fileserver),
		Clipboard:  fn(ip, clipboard),
	}

	if caps.VNC {
		hp.VNC = fn(ip, vnc)
	}

	return hp
}

func waitForPodToBeReady(client *kubernetes.Clientset, pod *apiv1.Pod, ns, name string, timeout time.Duration) error {
	status := pod.Status
	w, err := client.CoreV1().Pods(ns).Watch(metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
	})
	if err != nil {
		return err
	}
	func() {
		for {
			select {
			case events, ok := <-w.ResultChan():
				if !ok {
					return
				}
				resp := events.Object.(*apiv1.Pod)
				status = resp.Status
				if resp.Status.Phase != apiv1.PodPending {
					w.Stop()
				}
			case <-time.After(timeout):
				w.Stop()
			}
		}
	}()
	if status.Phase != apiv1.PodRunning {
		return fmt.Errorf("Pod is unavailable: %v", status.Phase)
	}
	return nil
}

func getEnvVars(service ServiceBase, caps session.Caps) []apiv1.EnvVar {
	envVar := []apiv1.EnvVar{
		{
			Name:  "TZ",
			Value: fmt.Sprintf("%s", getTimeZone(service, caps)),
		},
		{
			Name:  "SCREEN_RESOLUTION",
			Value: fmt.Sprintf("%s", caps.ScreenResolution),
		},
		{
			Name:  "ENABLE_VNC",
			Value: fmt.Sprintf("%v", caps.VNC),
		},
	}
	return envVar
}

func getResourses(service ServiceBase) apiv1.ResourceRequirements {
	res := apiv1.ResourceRequirements{}
	req := service.Service.Requirements
	if len(req.Limits) != 0 {
		res.Limits = getLimits(req.Limits)
	}
	if len(req.Requests) != 0 {
		res.Requests = getLimits(req.Requests)
	}
	return res
}

func getLimits(req map[string]string) apiv1.ResourceList {
	res := apiv1.ResourceList{}
	if cpu, ok := req["cpu"]; ok {
		res[apiv1.ResourceCPU] = resource.MustParse(cpu)
	}
	if mem, ok := req["memory"]; ok {
		res[apiv1.ResourceMemory] = resource.MustParse(mem)
	}
	return res
}

func getEmptyDirSizeLimit(service *config.Browser) *resource.Quantity {
	shm := resource.Quantity{}
	const unit = 1024
	if service.ShmSize < unit {
		shm = resource.MustParse(sizeLimitDefault)
	} else {
		div, exp := int64(unit), 0
		for n := service.ShmSize / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		shmSize := fmt.Sprintf("%.0f%ci", float64(service.ShmSize)/float64(div), "KMGTPE"[exp])
		shm = resource.MustParse(shmSize)
	}
	return &shm
}

func getContainerPort() []apiv1.ContainerPort {
	cp := []apiv1.ContainerPort{}
	fn := func(p apiv1.ContainerPort) {
		cp = append(cp, p)
	}
	fn(apiv1.ContainerPort{Name: "selenium", ContainerPort: selenium})
	fn(apiv1.ContainerPort{Name: "fileserver", ContainerPort: fileserver})
	fn(apiv1.ContainerPort{Name: "clipboard", ContainerPort: clipboard})
	fn(apiv1.ContainerPort{Name: "vnc", ContainerPort: vnc})
	return cp
}

func int64ToHuman(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f%ci", float64(b)/float64(div), "KMGTPE"[exp])
}
