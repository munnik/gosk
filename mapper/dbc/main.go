package dbc

import (
	"io/ioutil"
	"os"

	"github.com/munnik/gosk/logger"
	"go.einride.tech/can/pkg/dbc"
)

// func main() {
// 	messages := NewDBC("/home/albert/Documents/FuelEssence/TelMA_ID0x100_mit_Diagnosedate.dbc")
// 	fmt.Println(messages)
// 	fmt.Println(messages.messages[256])
// }

type DBC map[uint32]*dbc.MessageDef

func NewDBC(filename string) DBC {
	file, err := os.Open(filename)
	if err != nil {
		logger.GetLogger().Error(err.Error())
	}
	defer file.Close()
	source, err := ioutil.ReadAll(file)
	if err != nil {
		logger.GetLogger().Error(err.Error())
	}
	parser := dbc.NewParser(file.Name(), source)
	parser.Parse()
	messages := make(map[uint32]*dbc.MessageDef)
	for _, def := range parser.Defs() {
		switch def := def.(type) {
		case *dbc.MessageDef:
			id := def.MessageID
			messages[uint32(id)] = def
			// fmt.struct { {n(def)

		}

	}
	return messages
}
