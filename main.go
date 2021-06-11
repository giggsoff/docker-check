package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"github.com/lf-edge/eve/libs/zedUpload"
)

func main() {

	workers := 5
	iter := uint64(0)

	rand.Seed(time.Now().UnixNano())

	dCtx, err := zedUpload.NewDronaCtx("zdownloader", 0)
	if err != nil {
		log.Fatalf("NewDronaCtx: %s", err)
	}
	lock := sync.Mutex{}
	errorChan := make(chan error)
	for i := 0; i < workers; i++ {
		go func() {
			for {
				n := rand.Intn(5000)
				time.Sleep(time.Duration(n) * time.Millisecond)
				lock.Lock()
				iter++
				lock.Unlock()
				log.Printf("pull %d start", iter)
				if err := doPull(dCtx); err != nil {
					errorChan <- fmt.Errorf("doPull: %s", err)
				}
				log.Printf("pull %d end", iter)
			}
		}()
	}
	<-errorChan
}

func doPull(dCtx *zedUpload.DronaCtx) error {
	file, err := ioutil.TempFile("", "blobs")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(file.Name())

	ipSrc := net.ParseIP("")
	dEndPoint, err := dCtx.NewSyncerDest(zedUpload.SyncOCIRegistryTr, "index.docker.io", "itmoeve/eclient@sha256:57a7e84f11b2df67e5c485852c2dbd08c678b51ed69043152829a28216c88d9d", nil)
	if err != nil {
		return fmt.Errorf("NewSyncerDest: %s", err)
	}
	if err := dEndPoint.WithSrcIPSelection(ipSrc); err != nil {
		return fmt.Errorf("WithSrcIPSelection: %s", err)
	}

	var respChan = make(chan *zedUpload.DronaRequest)
	req := dEndPoint.NewRequest(zedUpload.SyncOpDownload, "", file.Name(), 0, true, respChan)
	if req == nil {
		return errors.New("NewRequest nil")
	}

	req = req.WithCancel(context.Background())
	defer req.Cancel()

	if err := req.Post(); err != nil {
		return fmt.Errorf("Post: %s", err)
	}

	for resp := range respChan {
		if resp.IsDnUpdate() {
			currentSize, totalSize, progress := resp.Progress()
			log.Printf("Update progress(%d) for %v: %v/%v", progress,
				resp.GetLocalName(), currentSize, totalSize)
			if currentSize > totalSize {
				errStr := fmt.Sprintf("Size '%v' provided in image config of '%s' is incorrect.\nDownload status (%v / %v). Aborting the download",
					totalSize, resp.GetLocalName(), currentSize, totalSize)
				return errors.New(errStr)
			}
			continue
		}
		err = resp.GetDnStatus()
		if resp.IsError() {
			return fmt.Errorf("GetDnStatus: %s", err)
		}
		log.Printf("Done for %v size %d",
			resp.GetLocalName(), resp.GetAsize())
		return nil
	}
	return errors.New("respChan EOF")
}
