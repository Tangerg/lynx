package json

import (
	"testing"
	"time"
)

type Addr struct {
	Zip      string `json:"zip"`
	Position string `json:"position"`
}
type TestUser struct {
	ID          int                    `json:"id"`
	Name        string                 `json:"name" jsonschema:"title=the name,description=The name of a friend,example=joe,example=lucy,default=alex"`
	Friends     []int                  `json:"friends,omitempty" jsonschema_description:"The list of IDs, omitted when empty"`
	Tags        map[string]interface{} `json:"tags,omitempty" jsonschema_extras:"a=b,foo=bar,foo=bar1"`
	BirthDate   time.Time              `json:"birth_date,omitempty" jsonschema:"oneof_required=date"`
	YearOfBirth string                 `json:"year_of_birth,omitempty" jsonschema:"oneof_required=year"`
	Metadata    interface{}            `json:"metadata,omitempty" jsonschema:"oneof_type=string;array"`
	FavColor    string                 `json:"fav_color,omitempty" jsonschema:"enum=red,enum=green,enum=blue"`
	Addrs       []*Addr                `json:"addrs,omitempty"`
}

func TestStringDefSchemaOf1(t *testing.T) {
	rv := StringDefSchemaOf(&TestUser{})
	t.Log(rv)
}

func TestStringDefSchemaOf2(t *testing.T) {
	rv := StringDefSchemaOf(Addr{})
	t.Log(rv)
}

func TestStringDefSchemaOf3(t *testing.T) {
	rv := StringDefSchemaOf(1)
	t.Log(rv)
}
