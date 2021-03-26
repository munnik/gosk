package signalkws

import (
	"time"
)

type helloMessage struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Self      string    `json:"self"`
	Roles     []string  `json:"roles"`
	Timestamp time.Time `json:"timestamp"`
}

type subscribeMessage struct {
	Context     string             `json:"context"`
	Subscribe   []subscribeSection `json:"subscribe"`
	Unsubscribe []subscribeSection `json:"unsubscribe"`
}

type fullMessage struct {
	Version string                   `json:"version"`
	Self    string                   `json:"self"`
	Vessels map[string]vesselSection `json:"vessels"`
}

type deltaMessage struct {
	Context string          `json:"context"`
	Updates []updateSection `json:"updates"`
}

type subscribeSection struct {
	Path string `json:"path"`
}

type vesselSection struct {
}

type updateSection struct {
	Source    sourceSection  `json:"source"`
	Timestamp time.Time      `json:"timestmap"`
	Values    []valueSection `json:"values"`
}

type sourceSection struct {
	Label string `json:"label"`
}

type valueSection struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}
