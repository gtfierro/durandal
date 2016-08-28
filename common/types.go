package common

import (
	"gopkg.in/mgo.v2/bson"
)

type Dict map[string]interface{}

func (d Dict) ToBSON() bson.M {
	var ret bson.M
	for k, v := range d {
		switch t := v.(type) {
		case string, int:
			ret[k] = v
		case Dict:
			ret[k] = t.ToBSON()
		}
	}
	return ret
}