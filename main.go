package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docker/go-units"
	"golang.org/x/net/websocket"

	"fmt"

	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/protect"
	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	"github.com/docker/docker/client"
	"path/filepath"
	"github.com/aerokube/selenoid/mesos/scheduler"
)

type memLimit int64

func (limit *memLimit) String() string {
	return units.HumanSize(float64(*limit))
}

func (limit *memLimit) Set(s string) error {
	v, err := units.RAMInBytes(s)
	if err != nil {
		return fmt.Errorf("set memory limit: %v", err)
	}
	*limit = memLimit(v)
	return nil
}

type cpuLimit int64

func (limit *cpuLimit) String() string {
	return strconv.FormatFloat(float64(*limit/1000000000), 'f', -1, 64)
}

func (limit *cpuLimit) Set(s string) error {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("set cpu limit: %v", err)
	}
	*limit = cpuLimit(v * 1000000000)
	return nil
}

var (
	hostname                 string
	disableDocker            bool
	disableQueue             bool
	enableFileUpload         bool
	listen                   string
	timeout                  time.Duration
	newSessionAttemptTimeout time.Duration
	sessionDeleteTimeout     time.Duration
	serviceStartupTimeout    time.Duration
	limit                    int
	retryCount               int
	containerNetwork         string
	sessions                 = session.NewMap()
	confPath                 string
	logConfPath              string
	captureDriverLogs        bool
	disablePrivileged        bool
	videoOutputDir           string
	videoRecorderImage       string
	conf                     *config.Config
	queue                    *protect.Queue
	manager                  service.Manager
	cli                      *client.Client
	mesosMasterURL           string

	startTime = time.Now()

	version     bool
	gitRevision string = "HEAD"
	buildStamp  string = "unknown"
)

func init() {
	var mem memLimit
	var cpu cpuLimit
	flag.BoolVar(&disableDocker, "disable-docker", false, "Disable docker support")
	flag.BoolVar(&disableQueue, "disable-queue", false, "Disable wait queue")
	flag.BoolVar(&enableFileUpload, "enable-file-upload", false, "File upload support")
	flag.StringVar(&listen, "listen", ":4444", "Network address to accept connections")
	flag.StringVar(&confPath, "conf", "config/browsers.json", "Browsers configuration file")
	flag.StringVar(&logConfPath, "log-conf", "config/container-logs.json", "Container logging configuration file")
	flag.IntVar(&limit, "limit", 5, "Simultaneous container runs")
	flag.IntVar(&retryCount, "retry-count", 1, "New session attempts retry count")
	flag.DurationVar(&timeout, "timeout", 60*time.Second, "Session idle timeout in time.Duration format")
	flag.DurationVar(&newSessionAttemptTimeout, "session-attempt-timeout", 30*time.Second, "New session attempt timeout in time.Duration format")
	flag.DurationVar(&sessionDeleteTimeout, "session-delete-timeout", 30*time.Second, "Session delete timeout in time.Duration format")
	flag.DurationVar(&serviceStartupTimeout, "service-startup-timeout", 30*time.Second, "Service startup timeout in time.Duration format")
	flag.BoolVar(&version, "version", false, "Show version and exit")
	flag.Var(&mem, "mem", "Containers memory limit e.g. 128m or 1g")
	flag.Var(&cpu, "cpu", "Containers cpu limit as float e.g. 0.2 or 1.0")
	flag.StringVar(&containerNetwork, "container-network", service.DefaultContainerNetwork, "Network to be used for containers")
	flag.BoolVar(&captureDriverLogs, "capture-driver-logs", false, "Whether to add driver process logs to Selenoid output")
	flag.BoolVar(&disablePrivileged, "disable-privileged", false, "Whether to disable privileged container mode")
	flag.StringVar(&videoOutputDir, "video-output-dir", "video", "Directory to save recorded video to")
	flag.StringVar(&videoRecorderImage, "video-recorder-image", "selenoid/video-recorder", "Image to use as video recorder")
	flag.StringVar(&mesosMasterURL, "mesos", "", "URL to mesos master")
	flag.Parse()

	if version {
		showVersion()
		os.Exit(0)
	}

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		log.Fatalf("%s: %v", os.Args[0], err)
	}
	queue = protect.New(limit, disableQueue)
	conf = config.NewConfig()
	err = conf.Load(confPath, logConfPath)
	if err != nil {
		log.Fatalf("%s: %v", os.Args[0], err)
	}
	onSIGHUP(func() {
		err := conf.Load(confPath, logConfPath)
		if err != nil {
			log.Printf("%s: %v", os.Args[0], err)
		}
	})
	cancelOnSignal()
	inDocker := false
	_, err = os.Stat("/.dockerenv")
	if err == nil {
		inDocker = true
	}
	videoOutputDir, err = filepath.Abs(videoOutputDir)
	if err != nil {
		log.Fatalf("Invalid video output dir %s: %v", videoOutputDir, err)
	}
	err = os.MkdirAll(videoOutputDir, os.FileMode(0644))
	if err != nil {
		log.Fatalf("Failed to create video output dir %s: %v", videoOutputDir, err)
	}
	environment := service.Environment{
		InDocker:            inDocker,
		CPU:                 int64(cpu),
		Memory:              int64(mem),
		Network:             containerNetwork,
		StartupTimeout:      serviceStartupTimeout,
		CaptureDriverLogs:   captureDriverLogs,
		VideoOutputDir:      videoOutputDir,
		VideoContainerImage: videoRecorderImage,
		Privileged:          !disablePrivileged,
		MesosMasterUrl:      mesosMasterURL,
	}
	if disableDocker {
		manager = &service.DefaultManager{Environment: &environment, Config: conf}
		return
	}
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = client.DefaultDockerHost
	}
	_, addr, _, err := client.ParseHost(dockerHost)
	if err != nil {
		log.Fatal(err)
	}
	ip, _, _ := net.SplitHostPort(addr)
	environment.IP = ip
	cli, err = client.NewEnvClient()
	if err != nil {
		log.Fatalf("new docker client: %v\n", err)
	}
	manager = &service.DefaultManager{Environment: &environment, Client: cli, Config: conf}

	if mesosMasterURL != "" {
		log.Printf("[TRY TO REGISTER ON MESOS MASTER] [%s]", mesosMasterURL)
		go scheduler.Run(mesosMasterURL)
	}

}

func cancelOnSignal() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		sessions.Each(func(k string, s *session.Session) {
			if enableFileUpload {
				os.RemoveAll(path.Join(os.TempDir(), k))
			}
			s.Cancel()
		})
		if !disableDocker {
			err := cli.Close()
			if err != nil {
				log.Fatalf("close docker client: %v", err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}()
}

func onSIGHUP(fn func()) {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGHUP)
	go func() {
		for {
			<-sig
			fn()
		}
	}()
}

func mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", queue.Check(queue.Protect(post(create))))
	mux.HandleFunc("/session/", proxy)
	return mux
}

func post(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func ping(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(struct {
		Uptime         string `json:"uptime"`
		LastReloadTime string `json:"lastReloadTime"`
		NumRequests    uint64 `json:"numRequests"`
		Version        string `json:"version"`
	}{time.Since(startTime).String(), conf.LastReloadTime.String(), getSerial(), gitRevision})
}

const videoPath = "/video/"

func video(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		deleteFileIfExists(w, r)
		return
	}
	fileServer := http.StripPrefix(videoPath, http.FileServer(http.Dir(videoOutputDir)))
	fileServer.ServeHTTP(w, r)
}

func deleteFileIfExists(w http.ResponseWriter, r *http.Request) {
	fileName := strings.TrimPrefix(r.URL.Path, videoPath)
	filePath := filepath.Join(videoOutputDir, fileName)
	_, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unknown file %s", filePath), http.StatusNotFound)
		return
	}
	err = os.Remove(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete file %s: %v", filePath, err), http.StatusInternalServerError)
		return
	}
	log.Printf("[%d] [DELETED_VIDEO_FILE] [%s]\n", serial(), fileName)
}

func handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("/wd/hub/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		r.URL.Scheme = "http"
		r.URL.Host = (&request{r}).localaddr()
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
		mux().ServeHTTP(w, r)
	})
	root.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		jsonError(w, "Session timed out or not found", http.StatusNotFound)
	})
	root.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(conf.State(sessions, limit, queue.Queued(), queue.Pending()))
	})
	root.HandleFunc("/ping", ping)
	root.Handle("/vnc/", websocket.Handler(vnc))
	root.Handle("/logs/", websocket.Handler(logs))
	root.HandleFunc(videoPath, video)
	if enableFileUpload {
		root.HandleFunc("/file", fileUpload)
	}
	return root
}

func showVersion() {
	fmt.Printf("Git Revision: %s\n", gitRevision)
	fmt.Printf("UTC Build Time: %s\n", buildStamp)
}

func main() {
	log.Printf("Timezone: %s\n", time.Local)
	log.Printf("Video Dir: %s\n", videoOutputDir)
	log.Printf("Listening on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, handler()))
}
