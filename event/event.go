package event

import "github.com/aerokube/selenoid/session"

var (
	fileCreatedListeners    []FileCreatedListener
	sessionStoppedListeners []SessionStoppedListener
)

type InitRequired interface {
	Init()
}

type Event struct {
	RequestId uint64
	SessionId string
	Session   *session.Session
}

type CreatedFile struct {
	Event
	Name string
	Type string
}

type FileCreatedListener interface {
	OnFileCreated(createdFile CreatedFile)
}

type StoppedSession struct {
	Event
}

type SessionStoppedListener interface {
	OnSessionStopped(stoppedSession StoppedSession)
}

func FileCreated(createdFile CreatedFile) {
	for _, l := range fileCreatedListeners {
		go l.OnFileCreated(createdFile)
	}
}

func InitIfNeeded(listener interface{}) {
	if l, ok := listener.(InitRequired); ok {
		l.Init()
	}
}

func AddFileCreatedListener(listener FileCreatedListener) {
	InitIfNeeded(listener)
	fileCreatedListeners = append(fileCreatedListeners, listener)
}

func SessionStopped(stoppedSession StoppedSession) {
	for _, l := range sessionStoppedListeners {
		go l.OnSessionStopped(stoppedSession)
	}
}

func AddSessionStoppedListener(listener SessionStoppedListener) {
	InitIfNeeded(listener)
	sessionStoppedListeners = append(sessionStoppedListeners, listener)
}
