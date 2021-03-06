package archiver

import (
	"github.com/gtfierro/pundat/common"
	uuidlib "github.com/pborman/uuid"
	"github.com/pkg/errors"
	btrdb "gopkg.in/btrdb.v3"
	"math/rand"
	"net"
	"sync"
	"time"
)

type btrdbConfig struct {
	address *net.TCPAddr
}

var BtrDBReadErr = errors.New("Error receiving data from BtrDB")

const MaximumTime = (48 << 56)

type btrIface struct {
	address *net.TCPAddr
	client  *btrdb.BTrDBConnection
	clients []*btrdb.BTrDBConnection
	sync.RWMutex
}

func newBtrIface(c *btrdbConfig) *btrIface {
	rand.Seed(time.Now().UnixNano())
	var err error
	b := &btrIface{
		address: c.address,
		clients: make([]*btrdb.BTrDBConnection, 10),
	}
	log.Noticef("Connecting to BtrDB at %v...", b.address.String())

	if b.client, err = btrdb.NewBTrDBConnection(c.address.String()); err != nil {
		log.Fatalf("Could not connect to btrdb: %v", err)
	}

	for i := 0; i < 10; i++ {
		c, err := btrdb.NewBTrDBConnection(c.address.String())
		if err != nil {
			log.Fatalf("Could not connect to btrdb: %v", err)
		}
		b.clients[i] = c
	}
	log.Notice("...connected!")

	return b
}

func (bdb *btrIface) getClient() *btrdb.BTrDBConnection {
	return bdb.clients[rand.Intn(10)]
}

func (bdb *btrIface) AddReadings(ts common.Timeseries) error {
	var (
		parsed_uuid uuidlib.UUID
		err         error
	)

	// turn the string representation into UUID bytes
	parsed_uuid = uuidlib.UUID(ts.UUID)

	records := make([]btrdb.StandardValue, len(ts.Records))
	for i, rdg := range ts.Records {
		records[i] = btrdb.StandardValue{Time: rdg.Time.UnixNano(), Value: rdg.Value}
	}
	client := bdb.getClient()
	c, err := client.InsertValues(parsed_uuid, records, false)
	<-c
	return err
}

func (bdb *btrIface) numberResponseFromChan(c chan btrdb.StandardValue) common.Timeseries {
	var sr = common.Timeseries{
		Records: []*common.TimeseriesReading{},
	}
	for val := range c {
		sr.Records = append(sr.Records, &common.TimeseriesReading{Time: time.Unix(0, val.Time), Value: val.Value})
	}
	return sr
}

func (bdb *btrIface) statisticalResponseFromChan(c chan btrdb.StatisticalValue) common.StatisticTimeseries {
	var sr = common.StatisticTimeseries{
		Records: []*common.StatisticsReading{},
	}
	for val := range c {
		sr.Records = append(sr.Records, &common.StatisticsReading{Time: time.Unix(0, val.Time), Count: val.Count, Min: val.Min, Max: val.Max, Mean: val.Mean})
	}
	return sr
}

func (bdb *btrIface) queryNearestValue(uuids []common.UUID, start int64, backwards bool) ([]common.Timeseries, error) {
	var ret = make([]common.Timeseries, len(uuids))
	var results []chan btrdb.StandardValue
	var generations []chan uint64
	var reasons []chan string
	client := bdb.getClient()
	for _, uu := range uuids {
		uuid := uuidlib.UUID(uu)
		values, gens, reason, err := client.QueryNearestValue(uuid, start, backwards, 0)
		results = append(results, values)
		generations = append(generations, gens)
		reasons = append(reasons, reason)
		if err != nil {
			for _, c := range results {
				for _ = range c {
				}
			}
			for _, c := range generations {
				for _ = range c {
				}
			}
			for _, c := range reasons {
				for _ = range c {
				}
			}
			return ret, err
		}
	}
	for i, c := range results {
		sr := bdb.numberResponseFromChan(c)
		sr.UUID = uuids[i]
		sr.Generation = <-generations[i]
		ret[i] = sr
	}
	for _, c := range reasons {
		for _ = range c {
		}
	}
	return ret, nil
}

func (bdb *btrIface) Prev(uuids []common.UUID, start int64) ([]common.Timeseries, error) {
	return bdb.queryNearestValue(uuids, start, true)
}

func (bdb *btrIface) Next(uuids []common.UUID, start int64) ([]common.Timeseries, error) {
	return bdb.queryNearestValue(uuids, start, false)
}

func (bdb *btrIface) GetData(uuids []common.UUID, start, end int64) ([]common.Timeseries, error) {
	var ret = make([]common.Timeseries, len(uuids))
	var results []chan btrdb.StandardValue
	var generations []chan uint64
	var reasons []chan string
	client := bdb.getClient()
	for _, uu := range uuids {
		uuid := uuidlib.UUID(uu)
		values, gens, reason, err := client.QueryStandardValues(uuid, start, end, 0)
		results = append(results, values)
		generations = append(generations, gens)
		reasons = append(reasons, reason)
		if err != nil {
			for _, c := range results {
				for _ = range c {
				}
			}
			for _, c := range generations {
				for _ = range c {
				}
			}
			for _, c := range reasons {
				for _ = range c {
				}
			}
			return ret, err
		}
	}
	for i, c := range results {
		sr := bdb.numberResponseFromChan(c)
		sr.UUID = uuids[i]
		sr.Generation = <-generations[i]
		ret[i] = sr
	}
	for _, c := range reasons {
		for _ = range c {
		}
	}
	return ret, nil
}

func (bdb *btrIface) StatisticalData(uuids []common.UUID, pointWidth int, start, end int64) ([]common.StatisticTimeseries, error) {
	var ret = make([]common.StatisticTimeseries, len(uuids))
	var results []chan btrdb.StatisticalValue
	var generations []chan uint64
	var reasons []chan string
	client := bdb.getClient()
	for _, uu := range uuids {
		uuid := uuidlib.UUID(uu)
		values, gens, reason, err := client.QueryStatisticalValues(uuid, start, end, uint8(pointWidth), 0)
		results = append(results, values)
		generations = append(generations, gens)
		reasons = append(reasons, reason)
		if err != nil {
			for _, c := range results {
				for _ = range c {
				}
			}
			for _, c := range generations {
				for _ = range c {
				}
			}
			for _, c := range reasons {
				for _ = range c {
				}
			}
			return ret, err
		}
	}
	for i, c := range results {
		sr := bdb.statisticalResponseFromChan(c)
		sr.UUID = uuids[i]
		sr.Generation = <-generations[i]
		ret[i] = sr
	}
	for _, c := range reasons {
		for _ = range c {
		}
	}
	return ret, nil
}

func (bdb *btrIface) WindowData(uuids []common.UUID, width uint64, start, end int64) ([]common.StatisticTimeseries, error) {
	var ret = make([]common.StatisticTimeseries, len(uuids))
	var results []chan btrdb.StatisticalValue
	var generations []chan uint64
	var reasons []chan string
	client := bdb.getClient()
	for _, uu := range uuids {
		uuid := uuidlib.UUID(uu)
		values, gens, reason, err := client.QueryWindowValues(uuid, start, end, width, 0, 0)
		results = append(results, values)
		generations = append(generations, gens)
		reasons = append(reasons, reason)
		if err != nil {
			for _, c := range results {
				for _ = range c {
				}
			}
			for _, c := range generations {
				for _ = range c {
				}
			}
			for _, c := range reasons {
				for _ = range c {
				}
			}
			return ret, err
		}
	}
	for i, c := range results {
		sr := bdb.statisticalResponseFromChan(c)
		sr.UUID = uuids[i]
		sr.Generation = <-generations[i]
		ret[i] = sr
	}
	for _, c := range reasons {
		for _ = range c {
		}
	}
	return ret, nil
}

func (bdb *btrIface) ChangedRanges(uuids []common.UUID, from_gen, to_gen uint64, resolution uint8) ([]common.ChangedRange, error) {
	var ret = make([]common.ChangedRange, len(uuids))
	var ranges []chan btrdb.TimeRange
	var generations []chan uint64
	var reasons []chan string
	client := bdb.getClient()
	for _, uu := range uuids {
		uuid := uuidlib.UUID(uu)
		timeRange, generation, reason, err := client.QueryChangedRanges(uuid, from_gen, to_gen, resolution)
		ranges = append(ranges, timeRange)
		generations = append(generations, generation)
		reasons = append(reasons, reason)
		if err != nil {
			for _, c := range ranges {
				for _ = range c {
				}
			}
			for _, c := range generations {
				for _ = range c {
				}
			}
			for _, c := range reasons {
				for _ = range c {
				}
			}
			return ret, err
		}
	}
	for i, c := range ranges {
		cr := common.ChangedRange{
			UUID:   uuids[i],
			Ranges: []*common.TimeRange{},
		}
		for rng := range c {
			tr := &common.TimeRange{
				StartTime:  rng.StartTime,
				EndTime:    rng.EndTime,
				Generation: <-generations[i],
			}
			cr.Ranges = append(cr.Ranges, tr)
		}
		ret[i] = cr
	}
	for _, c := range reasons {
		for _ = range c {
		}
	}
	return ret, nil
}

func (bdb *btrIface) DeleteData(uuids []common.UUID, start int64, end int64) error {
	client := bdb.getClient()
	for _, uu := range uuids {
		uuid := uuidlib.UUID(uu)
		if _, err := client.DeleteValues(uuid, start, end); err != nil {
			return err
		}
	}
	return nil
}

func (bdb *btrIface) ValidTimestamp(time int64, uot common.UnitOfTime) bool {
	var err error
	if uot != common.UOT_NS {
		time, err = common.ConvertTime(time, uot, common.UOT_NS)
	}
	return time >= 0 && time <= MaximumTime && err == nil
}

// this is a no-op for btrdbv3
func (bdb *btrIface) StreamExists(uuid common.UUID) (bool, error) {
	return true, nil
}

// this is a no-op for btrdbv3
func (bdb *btrIface) RegisterStream(uuid common.UUID, uri, name string) error {
	return nil
}
