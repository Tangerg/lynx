package xml

import (
	"strings"
	"testing"
)

func TestNewStreamScanner(t *testing.T) {
	parser, err := NewStreamScanner(&StreamScannerConfig{
		OnText: func(s string) error {
			t.Log("ontext", s)
			return nil
		},
		Listeners: []*ElementListener{
			{
				Name: Name{
					Local: "name",
				},
				OnComplete: func(element Element) error {
					t.Log("oncomplete", element.String())
					return nil
				},
			},
			{
				Name: Name{
					Local: "person",
				},
				OnComplete: func(element Element) error {
					t.Log("oncomplete", element.String())
					return nil
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = parser.Scan(strings.NewReader(`Before text
		<person id="1">
			<name>ToM</name>
			<age>30</age>
		</person>
		Middle text
		<person id="2">
			<name>Bob</name>
			<age>25</age>
		</person>
		After text`))

}
