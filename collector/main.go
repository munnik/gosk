package collector

import "io"

// Collector interface
type Collector interface {
	Collect(io.Writer) error
}

const (
	// Topic is used by the collectors, first string is the protocol name from the mapper, second string is the name of the collector.
	Topic string = "collector/%s/%s\x00" // Null character separates from real message
)
