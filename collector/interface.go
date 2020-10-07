package collector

import "io"

// Collector interface
type Collector interface {
	Collect(io.Writer) error
}
