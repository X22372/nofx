package config

import (
	"bitget/constants"
)

const (
	BaseUrl = "https://api.bitget.com"
	WsUrl   = "wss://ws.bitget.com/mix/v1/stream"

	PASSPHRASE    = "autoorder"
	TimeoutSecond = 30
	SignType      = constants.SHA256
)

var (
	ApiKey       = ""
	ApiSecretKey = ""
)

func Init(apiKey, apiSecretKey string) {
	ApiKey = apiKey
	ApiSecretKey = apiSecretKey
}
