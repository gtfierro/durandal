package archiver

import (
	"encoding/json"
	"fmt"
	"github.com/gtfierro/giles2/common"
	bw2 "github.com/immesys/bw2bind"
	"math"
	"strings"
	"time"
)

const GilesQueryChangedRangesPIDString = "2.0.8.8"

var GilesQueryChangedRangesPID = bw2.FromDotForm(GilesQueryChangedRangesPIDString)

type KeyValueQuery struct {
	Query string
	Nonce uint32
}

func (msg KeyValueQuery) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.PONumGilesKeyValueQuery, msg)
	return
}

type QueryError struct {
	Query string
	Nonce uint32
	Error string
}

func (msg QueryError) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.PONumGilesQueryError, msg)
	return
}

func (msg QueryError) IsEmpty() bool {
	return msg.Error == ""
}

type QueryMetadataResult struct {
	Nonce uint32
	Data  []KeyValueMetadata
}

func (msg QueryMetadataResult) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.PONumGilesMetadataResponse, msg)
	return
}

func (msg QueryMetadataResult) Dump() string {
	var res []string
	for _, kv := range msg.Data {
		res = append(res, kv.Dump())
	}
	return "[\n" + strings.Join(res, ",\n") + "\n]"
}

func (msg QueryMetadataResult) IsEmpty() bool {
	return len(msg.Data) == 0
}

type QueryTimeseriesResult struct {
	Nonce uint32
	Data  []Timeseries
	Stats []Statistics
}

func (msg QueryTimeseriesResult) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.PONumGilesTimeseriesResponse, msg)
	return
}

func (msg QueryTimeseriesResult) Dump() string {
	var res []string
	for _, ts := range msg.Data {
		res = append(res, ts.Dump())
	}
	for _, ts := range msg.Stats {
		res = append(res, ts.Dump())
	}
	return "[\n" + strings.Join(res, ",\n") + "\n]"
}

func (msg QueryTimeseriesResult) DumpWithFormattedTime() string {
	var res []string
	for _, ts := range msg.Data {
		res = append(res, ts.DumpWithFormattedTime())
	}
	for _, ts := range msg.Stats {
		res = append(res, ts.DumpWithFormattedTime())
	}
	return "[\n" + strings.Join(res, ",\n") + "\n]"
}

func (msg QueryTimeseriesResult) IsEmpty() bool {
	return len(msg.Data) == 0 && len(msg.Stats) == 0
}

type QueryChangedResult struct {
	Nonce   uint32
	Changed []ChangedRange
}

func (msg QueryChangedResult) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(GilesQueryChangedRangesPID, msg)
	return
}

func (msg QueryChangedResult) Dump() string {
	var res []string
	for _, cr := range msg.Changed {
		res = append(res, cr.Dump())
	}
	return "[\n" + strings.Join(res, ",\n") + "\n]"
}

func (msg QueryChangedResult) IsEmpty() bool {
	return len(msg.Changed) == 0
}

type KeyValueMetadata struct {
	UUID     string
	Path     string
	Metadata map[string]interface{}
}

func (msg KeyValueMetadata) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.PONumGilesKeyValueMetadata, msg)
	return
}

func (msg KeyValueMetadata) Dump() string {
	var md = make(map[string]interface{})
	for k, v := range msg.Metadata {
		if vmap, ok := v.(map[interface{}]interface{}); ok {
			for kk, vv := range vmap {
				md[k+"/"+kk.(string)] = vv
			}
		} else {
			md[k] = v
		}
	}
	md["uuid"] = msg.UUID
	md["path"] = msg.Path
	if bytes, err := json.MarshalIndent(md, "", "  "); err != nil {
		log.Error(err)
		return fmt.Sprintf("%+v", md)
	} else {
		return string(bytes)
	}
}

type Timeseries struct {
	UUID       string
	Path       string
	Generation uint64
	Times      []uint64
	Values     []float64
}

func (msg Timeseries) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.PONumGilesTimeseries, msg)
	return
}

func (msg Timeseries) ToReadings() []common.Reading {
	lesserLength := int(math.Min(float64(len(msg.Times)), float64(len(msg.Values))))
	var res = make([]common.Reading, lesserLength)
	for idx := 0; idx < lesserLength; idx++ {
		res[idx] = &common.SmapNumberReading{Time: msg.Times[idx], Value: msg.Values[idx], UoT: common.GuessTimeUnit(msg.Times[idx])}
	}
	return res
}

func (msg Timeseries) Dump() string {
	var res [][]interface{}
	for i, time := range msg.Times {
		res = append(res, []interface{}{time, msg.Values[i]})
	}
	if bytes, err := json.MarshalIndent(map[string]interface{}{"UUID": msg.UUID, "Timeseries": res}, "", "  "); err != nil {
		return fmt.Sprintf("%+v", res)
	} else {
		return string(bytes)
	}
}

func (msg Timeseries) DumpWithFormattedTime() string {
	var res [][]interface{}
	for i, timestamp := range msg.Times {
		formattime := time.Unix(0, int64(timestamp))
		res = append(res, []interface{}{formattime, msg.Values[i]})
	}
	if bytes, err := json.MarshalIndent(map[string]interface{}{"UUID": msg.UUID, "Timeseries": res}, "", "  "); err != nil {
		return fmt.Sprintf("%+v", res)
	} else {
		return string(bytes)
	}
}

type Statistics struct {
	UUID       string
	Generation uint64
	Times      []uint64
	Count      []uint64
	Min        []float64
	Mean       []float64
	Max        []float64
}

func (msg Statistics) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.PONumGilesTimeseries, msg)
	return
}

func (msg Statistics) ToReadings() []common.Reading {
	lesserLength := int(math.Min(float64(len(msg.Times)), float64(len(msg.Count))))
	var res = make([]common.Reading, lesserLength)
	for idx := 0; idx < lesserLength; idx++ {
		res[idx] = &common.StatisticalNumberReading{Time: msg.Times[idx], UoT: common.GuessTimeUnit(msg.Times[idx]), Count: msg.Count[idx], Min: msg.Min[idx], Max: msg.Max[idx], Mean: msg.Mean[idx]}
	}
	return res
}

func (msg Statistics) Dump() string {
	var res [][]interface{}
	for i, time := range msg.Times {
		res = append(res, []interface{}{time, msg.Count[i], msg.Min[i], msg.Mean[i], msg.Max[i]})
	}
	if bytes, err := json.MarshalIndent(map[string]interface{}{"UUID": msg.UUID, "Generation": msg.Generation, "Timeseries": res}, "", "  "); err != nil {
		return fmt.Sprintf("%+v", res)
	} else {
		return string(bytes)
	}
}

func (msg Statistics) DumpWithFormattedTime() string {
	var res [][]interface{}
	for i, timestamp := range msg.Times {
		formattime := time.Unix(0, int64(timestamp))
		res = append(res, []interface{}{formattime, msg.Count[i], msg.Min[i], msg.Mean[i], msg.Max[i]})
	}
	if bytes, err := json.MarshalIndent(map[string]interface{}{"UUID": msg.UUID, "Generation": msg.Generation, "Timeseries": res}, "", "  "); err != nil {
		return fmt.Sprintf("%+v", res)
	} else {
		return string(bytes)
	}
}

type ChangedRange struct {
	UUID       string
	Generation uint64
	StartTime  int64
	EndTime    int64
}

func (msg ChangedRange) Dump() string {
	if bytes, err := json.MarshalIndent(map[string]interface{}{"UUID": msg.UUID, "Generation": msg.Generation, "Start": msg.StartTime, "End": msg.EndTime}, "", "  "); err != nil {
		return fmt.Sprintf("%+v", msg)
	} else {
		return string(bytes)
	}
}

type BWavable interface {
	ToMsgPackBW() bw2.PayloadObject
}
