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

	"path/filepath"

	"github.com/aerokube/selenoid/config"
	"github.com/aerokube/selenoid/protect"
	"github.com/aerokube/selenoid/service"
	"github.com/aerokube/selenoid/session"
	_ "github.com/aerokube/selenoid/upload"
	"github.com/aerokube/util"
	"github.com/aerokube/util/docker"
	"github.com/docker/docker/client"
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
	maxTimeout               time.Duration
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
	logOutputDir             string
	conf                     *config.Config
	queue                    *protect.Queue
	manager                  service.Manager
	cli                      *client.Client

	startTime = time.Now()

	version     bool
	gitRevision = "HEAD"
	buildStamp  = "unknown"
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
	flag.DurationVar(&maxTimeout, "max-timeout", 1*time.Hour, "Maximum valid session idle timeout in time.Duration format")
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
	flag.StringVar(&videoRecorderImage, "video-recorder-image", "selenoid/video-recorder:latest-release", "Image to use as video recorder")
	flag.StringVar(&logOutputDir, "log-output-dir", "", "Directory to save session log to")
	flag.Parse()

	if version {
		showVersion()
		os.Exit(0)
	}

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		log.Fatalf("[-] [INIT] [%s: %v]", os.Args[0], err)
	}
	queue = protect.New(limit, disableQueue)
	conf = config.NewConfig()
	err = conf.Load(confPath, logConfPath)
	if err != nil {
		log.Fatalf("[-] [INIT] [%s: %v]", os.Args[0], err)
	}
	onSIGHUP(func() {
		err := conf.Load(confPath, logConfPath)
		if err != nil {
			log.Printf("[-] [INIT] [%s: %v]", os.Args[0], err)
		}
	})
	cancelOnSignal()
	inDocker := false
	_, err = os.Stat("/.dockerenv")
	if err == nil {
		inDocker = true
	}

	if !disableDocker {
		videoOutputDir, err = filepath.Abs(videoOutputDir)
		if err != nil {
			log.Fatalf("[-] [INIT] [Invalid video output dir %s: %v]", videoOutputDir, err)
		}
		err = os.MkdirAll(videoOutputDir, os.FileMode(0644))
		if err != nil {
			log.Fatalf("[-] [INIT] [Failed to create video output dir %s: %v]", videoOutputDir, err)
		}
		log.Printf("[-] [INIT] [Video Dir: %s]", videoOutputDir)
	}
	if logOutputDir != "" {
		logOutputDir, err = filepath.Abs(logOutputDir)
		if err != nil {
			log.Fatalf("[-] [INIT] [Invalid log output dir %s: %v]", logOutputDir, err)
		}
		err = os.MkdirAll(logOutputDir, os.FileMode(0644))
		if err != nil {
			log.Fatalf("[-] [INIT] [Failed to create log output dir %s: %v]", logOutputDir, err)
		}
		log.Printf("[-] [INIT] [Logs Dir: %s]", logOutputDir)
	}

	environment := service.Environment{
		InDocker:             inDocker,
		CPU:                  int64(cpu),
		Memory:               int64(mem),
		Network:              containerNetwork,
		StartupTimeout:       serviceStartupTimeout,
		SessionDeleteTimeout: sessionDeleteTimeout,
		CaptureDriverLogs:    captureDriverLogs,
		VideoOutputDir:       videoOutputDir,
		VideoContainerImage:  videoRecorderImage,
		LogOutputDir:         logOutputDir,
		Privileged:           !disablePrivileged,
	}
	if disableDocker {
		manager = &service.DefaultManager{Environment: &environment, Config: conf}
		if logOutputDir != "" && captureDriverLogs {
			log.Fatalf("[-] [INIT] [In drivers mode only one of -capture-driver-logs and -log-output-dir flags is allowed]")
		}
		return
	}
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = client.DefaultDockerHost
	}
	u, err := client.ParseHostURL(dockerHost)
	if err != nil {
		log.Fatalf("[-] [INIT] [%v]", err)
	}
	ip, _, _ := net.SplitHostPort(u.Host)
	environment.IP = ip
	cli, err = docker.CreateCompatibleDockerClient(
		func(specifiedApiVersion string) {
			log.Printf("[-] [INIT] [Using Docker API version: %s]", specifiedApiVersion)
		},
		func(determinedApiVersion string) {
			log.Printf("[-] [INIT] [Your Docker API version is %s]", determinedApiVersion)
		},
		func(defaultApiVersion string) {
			log.Printf("[-] [INIT] [Did not manage to determine your Docker API version - using default version: %s]", defaultApiVersion)
		},
	)
	if err != nil {
		log.Fatalf("[-] [INIT] [New docker client: %v]", err)
	}
	manager = &service.DefaultManager{Environment: &environment, Client: cli, Config: conf}
}

func cancelOnSignal() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("[-] [-] [SHUTTING_DOWN] [-] [-] [-] [-] [-] [-] [-]")
		sessions.Each(func(k string, s *session.Session) {
			if enableFileUpload {
				os.RemoveAll(path.Join(os.TempDir(), k))
			}
			s.Cancel()
		})
		if !disableDocker {
			err := cli.Close()
			if err != nil {
				log.Fatalf("[-] [SHUTTING_DOWN] [Error closing docker client: %v]", err)
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
	mux.HandleFunc("/session", queue.Try(queue.Check(queue.Protect(post(create)))))
	mux.HandleFunc("/session/", proxy)
	mux.HandleFunc("/status", status)
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
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Uptime         string `json:"uptime"`
		LastReloadTime string `json:"lastReloadTime"`
		NumRequests    uint64 `json:"numRequests"`
		Version        string `json:"version"`
	}{time.Since(startTime).String(), conf.LastReloadTime.String(), getSerial(), gitRevision})
}

func video(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		deleteFileIfExists(w, r, videoOutputDir, videoPath, "DELETED_VIDEO_FILE")
		return
	}
	fileServer := http.StripPrefix(videoPath, http.FileServer(http.Dir(videoOutputDir)))
	fileServer.ServeHTTP(w, r)
}

func deleteFileIfExists(w http.ResponseWriter, r *http.Request, dir string, prefix string, status string) {
	fileName := strings.TrimPrefix(r.URL.Path, prefix)
	filePath := filepath.Join(dir, fileName)
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
	log.Printf("[%d] [%s] [%s]", serial(), status, fileName)
}

const (
	videoPath = "/video/"
	logsPath  = "/logs/"
	errorPath = "/error"
)

func handler() http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("/wd/hub/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		r.URL.Scheme = "http"
		r.URL.Host = (&request{r}).localaddr()
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/wd/hub")
		mux().ServeHTTP(w, r)
	})
	root.HandleFunc(errorPath, func(w http.ResponseWriter, r *http.Request) {
		util.JsonError(w, "Session timed out or not found", http.StatusNotFound)
	})
	root.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(conf.State(sessions, limit, queue.Queued(), queue.Pending()))
	})
	root.HandleFunc("/ping", ping)
	root.Handle("/vnc/", websocket.Handler(vnc))
	root.HandleFunc(logsPath, logs)
	root.HandleFunc(videoPath, video)
	root.HandleFunc("/download/", reverseProxy(func(sess *session.Session) string { return sess.HostPort.Fileserver }, "DOWNLOADING_FILE"))
	root.HandleFunc("/clipboard/", reverseProxy(func(sess *session.Session) string { return sess.HostPort.Clipboard }, "CLIPBOARD"))
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
	log.Printf("[-] [INIT] [Timezone: %s]", time.Local)
	log.Printf("[-] [INIT] [Listening on %s]", listen)
	log.Fatal(http.ListenAndServe(listen, handler()))
}
