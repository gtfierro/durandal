package archiver

import (
	"time"

	"github.com/gtfierro/ob"
	"github.com/gtfierro/pundat/common"
	bw2 "github.com/immesys/bw2bind"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
)

type Stream struct {
	// timeseries identifier
	//UUID     common.UUID
	uuidExpr []ob.Operation
	// immutable source of the stream. What the Archive Request points to.
	// This is what we subscribe to for data to archive (but not metadata)
	uri  string
	name string
	// list of Metadata URIs
	metadataURIs    []string
	inheritMetadata bool

	// following fields used for parsing received messages
	// the type of PO to extract
	po int
	// value expression
	valueExpr   []ob.Operation
	valueString string
	// time expression
	timeExpr  []ob.Operation
	timeParse string

	// following fields used for operation of the stream
	cancel       chan bool
	subscription chan *bw2.SimpleMessage
}

func (s *Stream) URI() string {
	return s.uri
}

//
func (s *Stream) startArchiving(timeseriesStore TimeseriesStore, metadataStore MetadataStore) {
	// TODO: consider having a set of worker threads for handling subscriptions.
	// If this is high-enough volume, then we may end up dropping some messages
	// Maybe make a super large buffer channel (e.g. 10000 messages?) Have one goroutine
	// dump into that channel, and then have a set of worker threads consume that. Need a way
	// of scaling up/down the processing of that channel
	go func() {
		// for each message we receive
		for msg := range s.subscription {
			// for each payload object in the message
			for _, po := range msg.POs {
				// skip if its not the PO we expect
				if !po.IsType(s.po, s.po) {
					continue
				}

				// unpack the message
				//TODO: cannot assume msgpack
				var thing interface{}
				err := po.(bw2.MsgPackPayloadObject).ValueInto(&thing)
				if err != nil {
					log.Error(errors.Wrap(err, "Could not unmarshal msgpack object"))
					continue
				}

				// extract the possible value
				value := ob.Eval(s.valueExpr, thing)
				if value == nil {
					continue
				}

				// extract the time
				timestamp := s.getTime(thing)

				// if we have an expression to extract a UUID, we use that
				var currentUUID common.UUID
				if len(s.uuidExpr) > 0 {
					currentUUID = ob.Eval(s.uuidExpr, thing).(common.UUID)
				} else {
					// generate the UUID for this message's URI, POnum and value expression (and the name, when we have it)
					currentUUID = common.ParseUUID(uuid.NewV3(NAMESPACE_UUID, msg.URI+po.GetPODotNum()+s.name).String())
				}
				ts := common.Timeseries{
					UUID:   currentUUID,
					SrcURI: msg.URI,
				}

				//	When we observe a UUID, we need to build up the associations to its metadata
				//	When I get a new UUID, with a URI, I need to find all of the Metadata rcords
				//	in the MD database that are prefixes of this URI (w/o !meta suffix) and add
				//	those associations in when we need to
				if err := metadataStore.MapURItoUUID(msg.URI, currentUUID); err != nil {
					log.Error(err)
					continue
				}

				if err := metadataStore.AddNameTag(s.name, currentUUID); err != nil {
					log.Error(err)
					continue
				}

				// generate the timeseries values from our extracted value, and then save it
				// test if the value is a list
				ts.Records = []*common.TimeseriesReading{}
				if value_list, ok := value.([]interface{}); ok {
					for _, _val := range value_list {
						value_f64, ok := _val.(float64)
						if !ok {
							if value_u64, ok := value.(uint64); ok {
								value_f64 = float64(value_u64)
							} else if value_i64, ok := value.(int64); ok {
								value_f64 = float64(value_i64)
							} else {
								log.Errorf("Value %+v was not a float64 (was %T)", value, value)
								continue
							}
						}
						ts.Records = append(ts.Records, &common.TimeseriesReading{Time: timestamp, Value: value_f64})
					}
				} else {
					value_f64, ok := value.(float64)
					if !ok {
						if value_u64, ok := value.(uint64); ok {
							value_f64 = float64(value_u64)
						} else if value_i64, ok := value.(int64); ok {
							value_f64 = float64(value_i64)
						} else {
							log.Errorf("Value %+v was not a float64 (was %T)", value, value)
							continue
						}
					}
					ts.Records = append(ts.Records, &common.TimeseriesReading{Time: timestamp, Value: value_f64})
				}

				// We will check the cache first (using new interface call into btrdb)
				// and create the stream object if it doesn't exist.
				if exists, err := timeseriesStore.StreamExists(currentUUID); err != nil {
					log.Error(errors.Wrapf(err, "Could not check stream exists (%s)", currentUUID.String()))
					continue
				} else if !exists {
					if err := timeseriesStore.RegisterStream(currentUUID, msg.URI, s.name); err != nil {
						log.Error(errors.Wrapf(err, "Could not create stream (%s %s %s)", currentUUID.String(), msg.URI, s.name))
						continue
					}
				}

				// now we can assume the stream exists and can write to it
				if err := timeseriesStore.AddReadings(ts); err != nil {
					log.Error(errors.Wrapf(err, "Could not write timeseries reading %+v", ts))
				}
			}
		}
	}()
}

func (s *Stream) getTime(thing interface{}) time.Time {
	if len(s.timeExpr) == 0 {
		return time.Now()
	}
	timeThing := ob.Eval(s.timeExpr, thing)
	timeString, ok := timeThing.(string)
	if ok {
		parsedTime, err := time.Parse(s.timeParse, timeString)
		if err != nil {
			return time.Now()
		}
		return parsedTime
	}

	timeNum, ok := timeThing.(uint64)
	if ok {
		uot := common.GuessTimeUnit(timeNum)
		i_ns, err := common.ConvertTime(timeNum, uot, common.UOT_NS)
		if err != nil {
			log.Error(err)
		}
		return time.Unix(0, int64(i_ns))
	}
	return time.Now()
}
