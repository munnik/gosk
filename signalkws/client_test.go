package signalkws

import (
	"testing"
)

func TestIsPatternMatch(t *testing.T) {
	tests := []struct {
		subject  string
		pattern  string
		expected bool
	}{
		{"vessels.urn:mrn:imo:mmsi:244770688", "*", true},
		{"vessels.urn:mrn:imo:mmsi:244770688", "vessels.*", true},
		{"vessels.urn:mrn:imo:mmsi:244770688", "vessels.*:mmsi:*", true},
		{"vessels.urn:mrn:imo:mmsi:244770688", "vessels.self", false},
		{"vessels.urn:mrn:imo:mmsi:244770688", "vessels.*:uuid:*", false},
		{"environment.water.temperature", "*", true},
		{"environment.water.temperature", "*.temperature", true},
		{"environment.water.temperature", "*.water.temperature", true},
		{"environment.water.temperature", "environment.*", true},
		{"environment.water.temperature", "environment.*.temperature", true},
	}

	for _, test := range tests {
		match := isPatternMatch([]rune(test.subject), []rune(test.pattern))
		if match != test.expected {
			t.Errorf("Expected %t but got %t. subject = %s, pattern = %s", test.expected, match, test.subject, test.pattern)
		}
	}
}

func TestSubscribe(t *testing.T) {
	tests := []struct {
		name           string
		messages       []subscribeMessage
		delta          deltaMessage
		expectedResult bool
	}{
		{
			"No subscribe messages",
			[]subscribeMessage{},
			deltaMessage{Context: "test1"},
			false,
		},
		{
			"Single subscribe message",
			[]subscribeMessage{
				{Context: "test1", Subscribe: []subscribeSection{{Path: "test1"}}},
			},
			deltaMessage{Context: "test1", Updates: []updateSection{{Values: []valueSection{{Path: "test1"}}}}},
			true,
		},
		{
			"Multiple subscribe message",
			[]subscribeMessage{
				{Context: "test1", Subscribe: []subscribeSection{{Path: "test1"}}},
				{Context: "test2", Subscribe: []subscribeSection{{Path: "test2"}}},
			},
			deltaMessage{Context: "test1", Updates: []updateSection{{Values: []valueSection{{Path: "test1"}}}}},
			true,
		},
		{
			"Multiple subscribe message, with unsubscribe",
			[]subscribeMessage{
				{Context: "test1", Subscribe: []subscribeSection{{Path: "test1"}}},
				{Context: "test2", Subscribe: []subscribeSection{{Path: "test2"}}},
				{Context: "test1", Unsubscribe: []subscribeSection{{Path: "test1"}}},
			},
			deltaMessage{Context: "test1", Updates: []updateSection{{Values: []valueSection{{Path: "test1"}}}}},
			false,
		},
	}

	for _, test := range tests {
		c := Client{}
		for _, message := range test.messages {
			c.handleSubscribeMessages(message)
		}
		if c.isSubscribedTo(test.delta) != test.expectedResult {
			t.Errorf("Test '%s' failed", test.name)
		}
	}
}
