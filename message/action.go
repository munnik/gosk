package message

import "github.com/google/uuid"

const (
	STATE_FAILED    = "FAILED"
	STATE_PENDING   = "PENDING"
	STATE_COMPLETED = "COMPLETED"

	STATUS_CODE_SUCCESSFUL              = 200 // the request was successful
	STATUS_CODE_INVALID_REQUEST         = 400 // something is wrong with the client's request
	STATUS_CODE_AUTHENTICATION_REQUIRED = 401 // the request has not been applied because it lacks valid authentication credentials
	STATUS_CODE_PERMISSION_DENIED       = 403 // the client does not have permission to make the request
	STATUS_CODE_UNSUPPORTED_REQUEST     = 405 // the server does not support the request
	STATUS_CODE_SERVER_SIDE_ISSUE       = 502 // something went wrong carrying out the request on the server side
	STATUS_CODE_TIMEOUT                 = 504 // timeout on the server side trying to carry out the request
)

type ActionRequestPut struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

type ActionRequest struct {
	Uuid uuid.UUID        `json:"requestId"`
	Put  ActionRequestPut `json:"put"`
}

func NewActionRequest(path string, value interface{}) *ActionRequest {
	return &ActionRequest{
		Uuid: uuid.New(),
		Put: ActionRequestPut{
			Path:  path,
			Value: value,
		},
	}
}

type ActionResponse struct {
	Uuid       uuid.UUID `json:"requestId"`
	State      string    `json:"state"`
	StatusCode int       `json:"statusCode"`
}

func NewActionResponse(actionRequest *ActionRequest) *ActionResponse {
	return &ActionResponse{
		Uuid: actionRequest.Uuid,
	}
}

func (ar *ActionResponse) WithState(state string) *ActionResponse {
	ar.State = state
	return ar
}

func (ar *ActionResponse) WithStatusCode(statusCode int) *ActionResponse {
	ar.StatusCode = statusCode
	return ar
}
