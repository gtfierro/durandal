package archiver

import (
	"encoding/base64"
	"github.com/gtfierro/durandal/common"
	ob "github.com/gtfierro/giles2/objectbuilder"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
	bw2 "gopkg.in/immesys/bw2bind.v5"
	"reflect"
	"strings"
	//	"time"
)

// takes care of handling/parsing archive requests
type viewManager struct {
	client *bw2.BW2Client
	store  MetadataStore
	ts     TimeseriesStore
	subber *metadatasubscriber
	// map of alias -> VK namespace
	namespaceAliases map[string]string
	requestHosts     *SynchronizedArchiveRequestMap
	requestURIs      *SynchronizedArchiveRequestMap
	muxer            *SubscriberMultiplexer
}

func newViewManager(client *bw2.BW2Client, store MetadataStore, ts TimeseriesStore, subber *metadatasubscriber) *viewManager {
	return &viewManager{
		client:           client,
		store:            store,
		ts:               ts,
		subber:           subber,
		namespaceAliases: make(map[string]string),
		requestHosts:     NewSynchronizedArchiveRequestMap(),
		requestURIs:      NewSynchronizedArchiveRequestMap(),
		muxer:            NewSubscriberMultiplexer(client),
	}
}

// Given a namespace, we subscribe to <ns>/*/!meta/giles. For each received message
// on the URI, we extract the list of ArchiveRequests
func (vm *viewManager) subscribeNamespace(ns string) {
	namespace := strings.TrimSuffix(ns, "/") + "/*/!meta/giles"

	ro, _, err := vm.client.ResolveRegistry(ns)
	if err != nil {
		log.Fatal(errors.Wrapf(err, "Could not resolve namespace %s", ns))
	}
	// OKAY so the issue here is that bw2's objects package is vendored, and runs into
	// conflict when used with the bw2bind package. So, we cannot import the objects
	// package. We only need the objects package to get the *objects.Entity object from
	// the RoutingObject interface we get from calling ResolveRegistry. The reason why we
	// need an Entity object is so we can call its GetVK() method to get the namespace VK
	// that is mapped to by the alias we threw into ResolveRegistry.
	// Because the underlying object actually is an entity object, we can use the reflect
	// package to just call the method directly without having to import the objects
	// package to do the type conversion (e.g. ro.(*object.Entity).GetVK()).
	// The rest is just reflection crap: call the method using f.Call() using []reflect.Value
	// to indicate an empty arguments list. We use [0] to get the first (and only) result,
	// and call .Bytes() to return the underlying byte array returned by GetVK(). We
	// then interpret it using base64 urlsafe encoding to get the string value.
	f := reflect.ValueOf(ro).MethodByName("GetVK")
	nsvk := base64.URLEncoding.EncodeToString(f.Call([]reflect.Value{})[0].Bytes())
	vm.namespaceAliases[namespace] = nsvk
	log.Noticef("Resolved alias %s -> %s", namespace, nsvk)
	log.Noticef("Subscribe to %s", namespace)
	sub, err := vm.client.Subscribe(&bw2.SubscribeParams{
		URI: namespace,
	})
	if err != nil {
		log.Fatal(errors.Wrapf(err, "Could not subscribe to namespace %s", namespace))
	}

	common.NewWorkerPool(sub, func(msg *bw2.SimpleMessage) {
		parts := strings.Split(msg.URI, "/")
		key := parts[len(parts)-1]
		if key != "giles" {
			return
		}
		var requests []*ArchiveRequest
		// find list of existing requests at the received URI. Given the list of ones
		// that are there NOW, we remove the extras
		for _, po := range msg.POs {
			if !po.IsTypeDF(bw2.PODFGilesArchiveRequest) {
				continue
			}
			var request = new(ArchiveRequest)
			err := po.(bw2.MsgPackPayloadObject).ValueInto(request)
			if err != nil {
				log.Error(errors.Wrap(err, "Could not parse Archive Request"))
				continue
			}
			if request.PO == 0 {
				log.Error(errors.Wrap(err, "Request contained no PO"))
				continue
			}
			if request.Value == "" {
				log.Error(errors.Wrap(err, "Request contained no Value expression"))
				continue
			}
			request.FromVK = msg.From
			if request.URI == "" { // no URI supplied
				request.URI = strings.TrimSuffix(request.URI, "!meta/giles")
				request.URI = strings.TrimSuffix(request.URI, "/")
			}
			if len(request.MetadataURIs) == 0 {
				request.MetadataURIs = []string{request.URI}
			}
			// TODO: does the FROM VK have permission to ask this?
			requests = append(requests, request)
		}
		// TODO: handle requests
		for _, request := range requests {
			if err := vm.HandleArchiveRequest(request); err != nil {
				log.Error(errors.Wrapf(err, "Could not handle archive request %+v", request))
			}
		}
	}, 1000).Start()

	// handle archive requests that have already existed
	query, err := vm.client.Query(&bw2.QueryParams{
		URI: namespace,
	})
	if err != nil {
		log.Error(errors.Wrap(err, "Could not subscribe"))
	}
	for msg := range query {
		sub <- msg
	}
}

func (vm *viewManager) HandleArchiveRequest(request *ArchiveRequest) error {
	//TODO: need a mapping from the archive
	// requests to the URI that provided them so that we
	// can detect when an archive request is removed
	if request.FromVK == "" {
		return errors.New("VK was empty in ArchiveRequest")
	}

	stream := &Stream{
		uri:    request.URI,
		cancel: make(chan bool),
	}

	stream.valueExpr = ob.Parse(request.Value)

	if request.UUID == "" {
		stream.UUID = common.ParseUUID(uuid.NewV3(NAMESPACE_UUID, request.URI+string(request.PO)+request.Value).String())
	} else {
		stream.uuidExpr = ob.Parse(request.UUID)
	}

	if request.Time != "" {
		stream.timeExpr = ob.Parse(request.Time)
	}

	//TODO: do we really need this?
	//if request.MetadataExpr != "" {
	//	stream.metadataExpr = ob.Parse(request.MetadataExpr)
	//}

	if request.InheritMetadata {
		for _, uri := range GetURIPrefixes(request.URI) {
			stream.metadata = append(stream.metadata, uri+"/!meta/+")
		}
	}
	for _, uri := range request.MetadataURIs {
		stream.metadata = append(stream.metadata, uri+"/!meta/+")
	}

	sub, err := vm.client.Subscribe(&bw2.SubscribeParams{
		URI: stream.uri,
	})
	if err != nil {
		return errors.Wrapf(err, "Could not subscribe to %s", stream.uri)
	}
	stream.subscription = sub

	for _, muri := range stream.metadata {
		vm.subber.requestSubscription(muri)
	}

	// indicate that we've gotten an archive request
	request.Dump()

	if err := vm.store.MapURItoUUID(stream.uri, stream.UUID); err != nil {
		return err
	}
	// now, we save the stream
	stream.startArchiving(vm.ts)

	return nil
}

// removes from the hostURI mapping all of those requests that aren't in recentRequests list
func (vm *viewManager) UpdateArchiveRequests(hostURI string, recentRequests []*ArchiveRequest) {
	var keepList = new(ArchiveRequestList)
	for _, req := range recentRequests {
		keepList.AddRequest(req)
	}

	currentList := vm.requestHosts.Get(hostURI)
	if currentList != nil {
		for _, req := range *currentList {
			if !keepList.Contains(req) {
				vm.requestURIs.RemoveEntry(req.URI, req)
				continue
			}
			keepList.AddRequest(req)
		}
	}
	vm.requestHosts.SetList(hostURI, keepList)
}

func (vm *viewManager) AddArchiveRequest(hostURI, archiveURI string, request *ArchiveRequest) {
	vm.requestHosts.Set(hostURI, request)
	vm.requestURIs.Set(archiveURI, request)
}

func (vm *viewManager) RemoveArchiveRequests(hostURI string) {
	requests := vm.requestHosts.Get(hostURI)
	if requests == nil {
		return
	}
	for _, request := range *requests {
		//request.cancel <- true
		vm.requestHosts.Del(hostURI)
		vm.requestURIs.RemoveEntry(request.URI, request)
	}
}

func (vm *viewManager) StopArchiving(archiveURI string) {
}