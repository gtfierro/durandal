package common

import (
	uuid "github.com/pborman/uuid"
	"gopkg.in/vmihailenco/msgpack.v2"
	"sync"
	"time"
)

var EmptyTimeseries = []Timeseries{}
var EmptyStatisticTimeseries = []StatisticTimeseries{}

type TimeseriesReading struct {
	// uint64 timestamp
	Time time.Time
	Unit UnitOfTime
	// value associated with this timestamp
	Value float64
}

func (s *TimeseriesReading) EncodeMsgpack(enc *msgpack.Encoder) error {
	return enc.Encode(s.Time, s.Value)
}

func (s *TimeseriesReading) DecodeMsgpack(enc *msgpack.Decoder) error {
	return enc.Decode(s.Time, s.Value)
}

type StatisticsReading struct {
	// uint64 timestamp
	Time  time.Time
	Unit  UnitOfTime
	Count uint64
	Min   float64
	Mean  float64
	Max   float64
}

type Timeseries struct {
	sync.RWMutex
	Records    []*TimeseriesReading
	Generation uint64
	SrcURI     string
	UUID       UUID
}

func (ts Timeseries) Copy() Timeseries {
	ts.Lock()
	newts := Timeseries{
		Generation: ts.Generation,
		SrcURI:     ts.SrcURI,
		UUID:       ts.UUID,
		Records:    make([]*TimeseriesReading, len(ts.Records)),
	}
	copy(newts.Records, ts.Records)
	ts.Unlock()
	return newts
}

// sort by timestamp
func (ts Timeseries) Len() int {
	return len(ts.Records)
}

func (ts Timeseries) Swap(i, j int) {
	ts.Records[i], ts.Records[j] = ts.Records[j], ts.Records[i]
}

func (ts Timeseries) Less(i, j int) bool {
	return ts.Records[i].Time.Before(ts.Records[j].Time)
}

func (ts *Timeseries) AddRecord(rec *TimeseriesReading) {
	ts.Lock()
	ts.Records = append(ts.Records, rec)
	ts.Unlock()
}

func (ts *Timeseries) Extend(newts Timeseries) {
	ts.Lock()
	if len(ts.UUID) == 0 {
		ts.UUID = newts.UUID
	}
	if !uuid.Equal(uuid.UUID(ts.UUID), uuid.UUID(newts.UUID)) {
		ts.Unlock()
		return
	}

	ts.Records = append(ts.Records, newts.Records...)
	if newts.Generation > ts.Generation {
		ts.Generation = newts.Generation
	}

	ts.Unlock()
}

func (ts *Timeseries) NumReadings() int {
	ts.RLock()
	defer ts.RUnlock()
	return len(ts.Records)
}

type StatisticTimeseries struct {
	sync.RWMutex
	Records    []*StatisticsReading
	Generation uint64
	SrcURI     string
	UUID       UUID
}

func (ts *StatisticTimeseries) AddRecord(rec *StatisticsReading) {
	ts.Lock()
	ts.Records = append(ts.Records, rec)
	ts.Unlock()
}

func (ts *StatisticTimeseries) Extend(newts StatisticTimeseries) {
	ts.Lock()
	if len(ts.UUID) == 0 {
		ts.UUID = newts.UUID
	}
	if !uuid.Equal(uuid.UUID(ts.UUID), uuid.UUID(newts.UUID)) {
		ts.Unlock()
		return
	}

	ts.Records = append(ts.Records, newts.Records...)
	if newts.Generation > ts.Generation {
		ts.Generation = newts.Generation
	}

	ts.Unlock()
}

func (ts *StatisticTimeseries) NumReadings() int {
	ts.RLock()
	defer ts.RUnlock()
	return len(ts.Records)
}

// sort by timestamp
func (ts StatisticTimeseries) Len() int {
	return len(ts.Records)
}

func (ts StatisticTimeseries) Swap(i, j int) {
	ts.Records[i], ts.Records[j] = ts.Records[j], ts.Records[i]
}

func (ts StatisticTimeseries) Less(i, j int) bool {
	return ts.Records[i].Time.Before(ts.Records[j].Time)
}

type TimeseriesDataGroup interface {
	NumReadings() int
}

// closed on start, open on end: [start, end)
type TimeRange struct {
	StartTime  int64
	EndTime    int64
	Generation uint64
}

type ChangedRange struct {
	Ranges []*TimeRange
	UUID   UUID
}
