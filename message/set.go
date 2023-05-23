package message

import "github.com/google/uuid"

type Set struct {
	Id    uuid.UUID
	Path  string
	Value interface{}
}
