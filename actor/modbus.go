package actor

import (
	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/expression"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
)

type ModbusSetter struct {
	config config.ActorConfig
	env    expression.ExpressionEnvironment
}

func (m *ModbusSetter) Set(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

// DoAct creates a raw modbus message based on the set message and the setter config
func (m *ModbusSetter) DoAct(actionRequest *message.ActionRequest) (*message.Raw, *message.ActionResponse) {
	actionResponse := message.NewActionResponse(actionRequest)
	vm := vm.VM{}
	for _, actionConfig := range m.config.Actions {
		if actionRequest.Put.Path != actionConfig.Path {
			continue
		}

		output, err := expression.RunExpr(vm, m.env, actionConfig)
		if err != nil {
			return nil, actionResponse.WithState(message.STATE_FAILED).WithStatusCode(message.STATUS_CODE_SERVER_SIDE_ISSUE)
		}
		if bytes, ok := output.([]byte); ok {
			return message.NewRaw().WithType(m.config.Protocol).WithValue(bytes),
				actionResponse.WithState(message.STATE_COMPLETED).WithStatusCode(message.STATUS_CODE_SUCCESSFUL)
		}
		return nil, actionResponse.WithState(message.STATE_FAILED).WithStatusCode(message.STATUS_CODE_SERVER_SIDE_ISSUE)
	}

	return nil, actionResponse.WithState(message.STATE_FAILED).WithStatusCode(message.STATUS_CODE_UNSUPPORTED_REQUEST)
}
