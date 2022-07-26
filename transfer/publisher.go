package transfer

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
)

type TransferPublisher struct {
	db         *database.PostgresqlDatabase
	mqttConfig *config.MQTTConfig
	mqttClient mqtt.Client
}

func NewTransferPublisher(dbc *config.PostgresqlConfig, mqttc *config.MQTTConfig) *TransferPublisher {
	return &TransferPublisher{db: database.NewPostgresqlDatabase(dbc), mqttConfig: mqttc}
}

// query database at interval
// ask clients via mqtt
// compare results
// if needed ask for resend of data

// check for successful transfer after timeout
// mark completed in db
