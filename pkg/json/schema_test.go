package json

import (
	"testing"
	"time"
)

type TestUser struct {
	ID          int                    `json:"id"`
	Name        string                 `json:"name" jsonschema:"title=the name,description=The name of a friend,example=joe,example=lucy,default=alex"`
	Friends     []int                  `json:"friends,omitempty" jsonschema_description:"The list of IDs, omitted when empty"`
	Tags        map[string]interface{} `json:"tags,omitempty" jsonschema_extras:"a=b,foo=bar,foo=bar1"`
	BirthDate   time.Time              `json:"birth_date,omitempty" jsonschema:"oneof_required=date"`
	YearOfBirth string                 `json:"year_of_birth,omitempty" jsonschema:"oneof_required=year"`
	Metadata    interface{}            `json:"metadata,omitempty" jsonschema:"oneof_type=string;array"`
	FavColor    string                 `json:"fav_color,omitempty" jsonschema:"enum=red,enum=green,enum=blue"`
}

func TestStringSchemaOf1(t *testing.T) {
	rv := StringSchemaOf(map[string]any{
		"a": "a",
		"b": 1,
	})
	t.Log(rv)
}
func TestStringSchemaOf2(t *testing.T) {
	rv := StringSchemaOf(false)
	t.Log(rv)
}
func TestStringSchemaOf3(t *testing.T) {
	rv := StringSchemaOf(123)
	t.Log(rv)
}
func TestStringSchemaOf4(t *testing.T) {
	rv := StringSchemaOf(123.123)
	t.Log(rv)
}
func TestStringSchemaOf5(t *testing.T) {
	rv := StringSchemaOf("string")
	t.Log(rv)
}
func TestStringSchemaOf6(t *testing.T) {
	rv := StringSchemaOf([]int{1, 2, 3})
	t.Log(rv)
}
func TestStringSchemaOf7(t *testing.T) {
	rv := StringSchemaOf([]string{"a", "b", "c"})
	t.Log(rv)
}
func TestStringSchemaOf8(t *testing.T) {
	rv := StringSchemaOf(&TestUser{})
	t.Log(rv)
}
func TestMapSchemaOf1(t *testing.T) {
	rv := MapSchemaOf(&TestUser{})
	t.Log(rv)
	t.Log(rv["$defs"])
}

func TestStringDefSchemaOf1(t *testing.T) {
	rv := StringDefSchemaOf(&TestUser{})
	t.Log(rv)
}
