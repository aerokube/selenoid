package upload

import (
	"github.com/aerokube/selenoid/event"
	"github.com/aerokube/util"
	"log"
	"sync"
	"time"
)

var (
	upl *Upload
)

type Uploader interface {
	Upload(createdFile event.CreatedFile) (bool, error)
}

type Upload struct {
	uploaders []Uploader
	lock      sync.Mutex
}

func AddUploader(u Uploader) {
	if upl == nil {
		upl = &Upload{}
		event.AddFileCreatedListener(upl)
	}
	upl.lock.Lock()
	defer upl.lock.Unlock()
	event.InitIfNeeded(u)
	upl.uploaders = append(upl.uploaders, u)
}

func (ul *Upload) OnFileCreated(createdFile event.CreatedFile) {
	if len(ul.uploaders) > 0 {
		for _, uploader := range ul.uploaders {
			go func() {
				s := time.Now()
				uploaded, err := uploader.Upload(createdFile)
				if err != nil {
					log.Printf("[%d] [UPLOADING_FILE] [%s] [Failed to upload: %v]", createdFile.RequestId, createdFile.Name, err)
					return
				}
				if uploaded {
					log.Printf("[%d] [UPLOADED_FILE] [%s] [%.2fs]", createdFile.RequestId, createdFile.Name, util.SecondsSince(s))
				}
			}()
		}
	}
}
