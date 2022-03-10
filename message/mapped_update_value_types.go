package message

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

type Position struct {
	Altitude  *float64 `json:"altitude,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

type Length struct {
	Overall   *float64 `json:"overall,omitempty"`
	Hull      *float64 `json:"hull,omitempty"`
	Waterline *float64 `json:"waterline,omitempty"`
}

type Alarm struct {
	State   bool   `json:"state"`
	Message string `json:"message,omitempty"`
}

func Decode(input interface{}) (interface{}, error) {
	if i, ok := input.(int64); ok {
		return i, nil
	}
	if f, ok := input.(float64); ok {
		return f, nil
	}
	if s, ok := input.(string); ok {
		return s, nil
	}

	var metadata mapstructure.Metadata
	p := Position{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &p, &metadata); err == nil && len(metadata.Unused) == 0 {
		return p, nil
	}

	l := Length{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &l, &metadata); err == nil && len(metadata.Unused) == 0 {
		return l, nil
	}

	a := Alarm{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &a, &metadata); err == nil && len(metadata.Unused) == 0 {
		return a, nil
	}

	return input, fmt.Errorf("don't know how to decode %v", input)
}
