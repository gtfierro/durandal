package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/gtfierro/bw2util"
	"github.com/gtfierro/pundat/common"
	bw2 "github.com/immesys/bw2bind"
	"github.com/op/go-logging"
)

// logger
var log *logging.Logger

var DB = NewPrefixDB(&Config{
	path: "pfx-leveldb",
})

// set up logging facilities
func init() {
	log = logging.MustGetLogger("scraper")
	var format = "%{color}%{level} %{shortfile} %{time:Jan 02 15:04:05} %{color:reset} ▶ %{message}"
	var logBackend = logging.NewLogBackend(os.Stderr, "", 0)
	logBackendLeveled := logging.AddModuleLevel(logBackend)
	logging.SetBackend(logBackendLeveled)
	logging.SetFormatter(logging.MustStringFormatter(format))
}

type listener struct {
	// initialization
	Client    *bw2util.Client
	Namespace string

	// internals
	// local buffer for messages to be processed
	msgBuffer    chan *bw2.SimpleMessage
	subscribeURI string
}

func (l *listener) init() {
	l.msgBuffer = make(chan *bw2.SimpleMessage)
	// build metadata subscription uri
	l.subscribeURI = l.Namespace + "/*/!meta/+"

	var (
		subc     chan *bw2.SimpleMessage
		subErr   error
		queryc   chan *bw2.SimpleMessage
		queryErr error
	)
	// subscribe to the namespace
	for {
		subc, subErr = l.Client.Subscribe(&bw2.SubscribeParams{
			URI: l.subscribeURI,
		})
		if subErr != nil {
			log.Error(subErr)
			time.Sleep(30 * time.Second) // retry in 30 seconds
		}
		break
	}

	// query the namespace to get persisted messages
	for {
		queryc, queryErr = l.Client.Query(&bw2.QueryParams{
			URI: l.subscribeURI,
		})
		if queryErr != nil {
			log.Error(queryErr)
			time.Sleep(30 * time.Second) // retry in 30 seconds
		}
		break
	}

	// start workers
	for w := 0; w < 10; w++ {
		go l.startWorker()
	}

	go func() {
		// add to buffer
		// TODO: add to a worker, if not, add to buffer
		for msg := range queryc {
			l.msgBuffer <- msg
		}
		log.Info("Finished adding Query msg")
		for msg := range subc {
			l.msgBuffer <- msg
		}
	}()

	return
}

func (l *listener) startWorker() {
	for msg := range l.msgBuffer {
		//uri := msg.URI
		mdobj := common.RecordFromMessageKey(msg)
		if len(mdobj.SrcURI) == 0 {
			log.Error("PODFSMetadata object was not a MetadataPayloadObject")
			continue
		}
		if err := DB.InsertRecords(mdobj); err != nil {
			log.Error(err)
			continue
		}
	}
}

func main() {
	_client := bw2.ConnectOrExit("")
	_client.OverrideAutoChainTo(true)
	vk := _client.SetEntityFromEnvironOrExit()

	client, err := bw2util.NewClient(_client, vk)
	if err != nil {
		log.Fatal(err)
	}

	namespaces := []string{"ucberkeley/eecs/sdh"}

	for _, ns := range namespaces {
		l := &listener{
			Client:    client,
			Namespace: ns,
		}
		l.init()
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("URI: ")
		text, _ := reader.ReadString('\n')
		for key, val := range DB.Lookup(text) {
			fmt.Println(">", key, "=", val)
		}
	}
}
