package upload

import (
	"github.com/aerokube/selenoid/session"
	"github.com/aerokube/util"
	"log"
	"time"
)

type UploadRequest struct {
	Filename string
	RequestId uint64
	SessionId string
	Session *session.Session
	Type string
}

type Uploader interface {
	Init()
	Upload(input *UploadRequest) error
}

var (
	uploader Uploader
)

func Init() {
	if uploader != nil {
		uploader.Init()
	}
}

func Upload(input *UploadRequest) {
	if uploader != nil {
		go func() {
			s := time.Now()
			err := uploader.Upload(input)
			if err != nil {
				log.Printf("[%d] [UPLOADING_FILE] [%s] [Failed to upload: %v]", input.RequestId, input.Filename, err)
				return
			}
			log.Printf("[%d] [UPLOADED_FILE] [%s] [%.2fs]", input.RequestId, input.Filename, util.SecondsSince(s))
		}()
	}
}
