package ratelimit

import (
	"go.nanomsg.org/mangos/v3"
)

type RateLimiter interface {
	RateLimit(subscriber mangos.Socket, publisher mangos.Socket)
}

// type RateLimiter interface {
// 	doForward(*message.SingleValueMapped) bool
// }
