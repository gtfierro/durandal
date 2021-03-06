package archiver

import (
	"github.com/gtfierro/pundat/common"
	"github.com/gtfierro/pundat/dots"
	"sort"
)

// how to do metadata DOT protection? run the query; if there is a uuid or path, we
// then see if we can build a chain to the path (or translate uuid into a uri); if that is
// the case, then we return, else we don't.

func (a *Archiver) SelectTags(vk string, params *common.TagParams) ([]common.MetadataGroup, error) {
	groups, err := a.MD.GetMetadata(vk, params.Tags, params.Where)
	if err != nil {
		return nil, err
	}
	return a.maskMetadataGroupsByPermission(vk, groups)
}

func (a *Archiver) DistinctTag(vk string, params *common.DistinctParams) ([]string, error) {
	return a.MD.GetDistinct(vk, params.Tag, params.Where)
}

func (a *Archiver) SelectDataRange(vk string, params *common.DataParams) ([]common.Timeseries, error) {
	var (
		err    error
		result []common.Timeseries
	)
	if err = a.prepareDataParams(params); err != nil {
		return result, err
	}
	result = make([]common.Timeseries, len(params.UUIDs))

	// TODO: this should be a time.Time consistently throughout (params. begin, end)
	requestedRange := dots.NewTimeRangeNano(params.Begin, params.End)

	// for each of the UUIDs in the params, get the intersection of that with the
	// valid ranges of access this VK has to that UUID.
	for idx, uuid := range params.UUIDs {
		uri, err := a.MD.URIFromUUID(uuid)
		if err != nil {
			return result, err
		}
		validRanges, err := a.dotmaster.GetValidRanges(uri, vk)
		if err != nil {
			return result, err
		}
		validRequestedRanges := validRanges.GetOverlap(requestedRange)
		for _, rng := range validRequestedRanges.Ranges {
			tsresult, err := a.TS.GetDataUUID(uuid, rng.Start.UnixNano(), rng.End.UnixNano(), params.ConvertToUnit)
			if err != nil {
				return result, err
			}
			result[idx].Extend(tsresult)

			// check limit
			if params.DataLimit > 0 && len(result[idx].Records) > params.DataLimit {
				result[idx].Records = result[idx].Records[:params.DataLimit]
				continue
			}
		}
	}

	return result, err
}

// selects the data point most immediately before the Start parameter for all matching streams
func (a *Archiver) SelectDataBefore(vk string, params *common.DataParams) (result []common.Timeseries, err error) {
	if err = a.prepareDataParams(params); err != nil {
		return
	}
	result, err = a.TS.Prev(params.UUIDs, params.Begin)
	result = a.packResults(params, result)
	return a.maskTimeseriesByPermission(vk, result)
}

// selects the data point most immediately after the Start parameter for all matching streams
func (a *Archiver) SelectDataAfter(vk string, params *common.DataParams) (result []common.Timeseries, err error) {
	if err = a.prepareDataParams(params); err != nil {
		return
	}
	result, err = a.TS.Next(params.UUIDs, params.Begin)
	result = a.packResults(params, result)
	return a.maskTimeseriesByPermission(vk, result)
}

func (a *Archiver) SelectStatisticalData(vk string, params *common.DataParams) (result []common.StatisticTimeseries, err error) {
	if err = a.prepareDataParams(params); err != nil {
		return
	}
	result = make([]common.StatisticTimeseries, len(params.UUIDs))
	requestedRange := dots.NewTimeRangeNano(params.Begin, params.End)

	for idx, uuid := range params.UUIDs {
		uri, err := a.MD.URIFromUUID(uuid)
		if err != nil {
			return result, err
		}
		validRanges, err := a.dotmaster.GetValidRanges(uri, vk)
		if err != nil {
			return result, err
		}
		validRequestedRanges := validRanges.GetOverlap(requestedRange)
		for _, rng := range validRequestedRanges.Ranges {

			var tsresult common.StatisticTimeseries
			if params.IsStatistical {
				tsresult, err = a.TS.StatisticalDataUUID(uuid, params.PointWidth, rng.Start.UnixNano(), rng.End.UnixNano(), params.ConvertToUnit)
			} else if params.IsWindow {
				tsresult, err = a.TS.WindowDataUUID(uuid, params.Width, rng.Start.UnixNano(), rng.End.UnixNano(), params.ConvertToUnit)
			}
			log.Debug(len(tsresult.Records))

			if err != nil {
				return result, err
			}
			result[idx].Extend(tsresult)

			// check limit
			if params.DataLimit > 0 && len(result[idx].Records) > params.DataLimit {
				result[idx].Records = result[idx].Records[:params.DataLimit]
				continue
			}
		}
	}

	return result, err
}

func (a *Archiver) GetChangedRanges(params *common.DataParams) (result []common.ChangedRange, err error) {
	if err = a.prepareDataParams(params); err != nil {
		return
	}
	result, err = a.TS.ChangedRanges(params.UUIDs, params.FromGen, params.ToGen, params.Resolution)
	return
}

func (a *Archiver) prepareDataParams(params *common.DataParams) (err error) {
	// parse and evaluate the where clause if we need to
	if len(params.Where) > 0 {
		params.UUIDs, err = a.MD.GetUUIDs("", params.Where)
		if err != nil {
			return err
		}
	}

	// apply the streamlimit if it exists
	if params.StreamLimit > 0 && len(params.UUIDs) > params.StreamLimit {
		params.UUIDs = params.UUIDs[:params.StreamLimit]
	}

	// make sure that Begin/End are both in nanoseconds
	if begin_uot := common.GuessTimeUnit(params.Begin); begin_uot != common.UOT_NS {
		params.Begin, err = common.ConvertTime(params.Begin, begin_uot, common.UOT_NS)
		if err != nil {
			return err
		}
	}
	if end_uot := common.GuessTimeUnit(params.End); end_uot != common.UOT_NS {
		params.End, err = common.ConvertTime(params.End, end_uot, common.UOT_NS)
		if err != nil {
			return err
		}
	}

	// switch order so its consistent
	if params.End < params.Begin {
		params.Begin, params.End = params.End, params.Begin
	}
	return nil
}

func (a *Archiver) packResults(params *common.DataParams, readings []common.Timeseries) []common.Timeseries {
	for i, resp := range readings {
		resp.Lock()
		if len(resp.Records) > 0 {
			// mark timestamps by how they should be transformed
			for idx, rdg := range resp.Records {
				rdg.Unit = params.ConvertToUnit
				resp.Records[idx] = rdg
			}
			readings[i] = resp
		}
		resp.Unlock()
	}
	log.Debugf("Returning %d readings", len(readings))
	return readings
}

func (a *Archiver) maskTimeseriesByPermission(vk string, readings []common.Timeseries) ([]common.Timeseries, error) {
	var (
		ret []common.Timeseries
	)
	// we want to mask the timeseries by the valid ranges
	for _, ts := range readings {
		// sort the timeseries by timestamp (earliest to most recent)
		sort.Sort(ts)
		uri, err := a.MD.URIFromUUID(ts.UUID)
		if err != nil {
			return common.EmptyTimeseries, err
		}
		// fetch the valid ranges for the URI that published these
		validRanges, err := a.dotmaster.GetValidRanges(uri, vk)
		if err != nil {
			return common.EmptyTimeseries, err
		}
		newts := &common.Timeseries{
			Generation: ts.Generation,
			SrcURI:     uri,
			UUID:       ts.UUID,
		}
		log.Infof("Got ranges (VK=%s, UUID=%s)%s", vk, ts.UUID, validRanges)
		for _, rng := range validRanges.Ranges {
			// find the first index of the timeseries record that is outside the lower bound
			earlyIndex := sort.Search(ts.Len(), func(idx int) bool {
				return ts.Records[idx].Time.Before(rng.Start)
			})
			// if we find no such index, then bound by our first reading
			if earlyIndex == ts.Len() {
				earlyIndex = 0
			}
			// find the first index of the timeseries record that is outside the upperbound
			// if we find no such index, then we are bound by our last reading
			lastIndex := sort.Search(ts.Len(), func(idx int) bool {
				return ts.Records[idx].Time.After(rng.End)
			})
			newts.Records = append(newts.Records, ts.Records[earlyIndex:lastIndex]...)
		}
		ret = append(ret, *newts)
	}
	return ret, nil
}

func (a *Archiver) maskMetadataGroupsByPermission(vk string, metadata []common.MetadataGroup) ([]common.MetadataGroup, error) {
	var (
		ret []common.MetadataGroup
	)
	for _, group := range metadata {
		// need to resolve path
		if group.URI == "" {
			log.Error("NULL URI")
			uri, err := a.MD.URIFromUUID(group.UUID)
			if err != nil {
				return ret, err
			}
			group.URI = uri
		}
		if err := a.dotmaster.CanRead(group.URI, vk); err != nil {
			continue
		}
		ret = append(ret, group)
	}
	return ret, nil
}
