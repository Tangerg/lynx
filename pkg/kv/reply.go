package kv

import (
	"encoding/json"
	"time"

	"github.com/spf13/cast"
)

type Reply struct {
	v any
}

func (r *Reply) Int() int {
	return cast.ToInt(r.v)
}
func (r *Reply) Int8() int8 {
	return cast.ToInt8(r.v)
}
func (r *Reply) Int16() int16 {
	return cast.ToInt16(r.v)
}
func (r *Reply) Int32() int32 {
	return cast.ToInt32(r.v)
}
func (r *Reply) Int64() int64 {
	return cast.ToInt64(r.v)
}
func (r *Reply) Uint() uint {
	return cast.ToUint(r.v)
}
func (r *Reply) Uint8() uint8 {
	return cast.ToUint8(r.v)
}
func (r *Reply) Uint16() uint16 {
	return cast.ToUint16(r.v)
}
func (r *Reply) Uint32() uint32 {
	return cast.ToUint32(r.v)
}
func (r *Reply) Uint64() uint64 {
	return cast.ToUint64(r.v)
}
func (r *Reply) Float32() float32 {
	return cast.ToFloat32(r.v)
}
func (r *Reply) Float64() float64 {
	return cast.ToFloat64(r.v)
}
func (r *Reply) String() string {
	return cast.ToString(r.v)
}
func (r *Reply) Slice() []any {
	return cast.ToSlice(r.v)
}
func (r *Reply) StringMap() map[string]any {
	return cast.ToStringMap(r.v)
}
func (r *Reply) Bool() bool {
	return cast.ToBool(r.v)
}
func (r *Reply) Time() time.Time {
	return cast.ToTime(r.v)
}
func (r *Reply) Duration() time.Duration {
	return cast.ToDuration(r.v)
}
func (r *Reply) Bytes() ([]byte, error) {
	return json.Marshal(r.v)
}
func (r *Reply) Unmarshal(v any) error {
	marshal, err := json.Marshal(r.v)
	if err != nil {
		return err
	}
	return json.Unmarshal(marshal, v)
}
