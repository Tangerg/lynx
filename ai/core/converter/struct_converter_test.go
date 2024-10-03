package converter

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

func TestStructConverter(t *testing.T) {
	s := new(StructConverter[TestUser])
	format := s.GetFormat()
	t.Log(format)
	s1 := new(StructConverter[bool])
	format1 := s1.GetFormat()
	t.Log(format1)
	s2 := new(StructConverter[int])
	format2 := s2.GetFormat()
	t.Log(format2)
	s3 := new(StructConverter[[]string])
	format3 := s3.GetFormat()
	t.Log(format3)
	s4 := new(StructConverter[map[string]any])
	format4 := s4.GetFormat()
	t.Log(format4)

	s5 := new(StructConverter[any])
	user := &TestUser{}
	s5.SetV(user)
	format5 := s5.GetFormat()
	t.Log(format5)

	s6 := new(StructConverter[any])
	var a any
	s5.SetV(a)
	format6 := s6.GetFormat()
	t.Log(format6)
}

func TestStructConverter2(t *testing.T) {
	s5 := new(StructConverter[any])
	user := &TestUser{}
	s5.SetV(user)
	format5 := s5.GetFormat()
	t.Log(format5)

	var a *TestUser
	s6 := new(StructConverter[any])
	s6.SetV(a)
	format6 := s6.GetFormat()
	t.Log(format6)
}
