package nanomsg

// Used identify header segments
const (
	HEADERSEGMENTPROCESS  = 0
	HEADERSEGMENTPROTOCOL = 1
	HEADERSEGMENTSOURCE   = 2
)

// Used to determine the datatype of the value
const (
	DOUBLE     = 0
	STRING     = 1
	POSITION   = 2
	LENGTH     = 3
	VESSELDATA = 4
)
