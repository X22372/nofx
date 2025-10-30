package trader

import (
	"bitget/config"
	"bitget/pkg/client"
	"encoding/json"
	"log"
	"strconv"

	"github.com/spf13/cast"
)

type BitgetTrader struct {
	client *client.BitgetApiClient
}

func NewBitgetTrader(apiKey, secretKey string) *BitgetTrader {
	config.Init(apiKey, secretKey)
	clt := new(client.BitgetApiClient).Init()
	return &BitgetTrader{client: clt}
}

func (t *BitgetTrader) GetBalance() (map[string]interface{}, error) {
	return nil, nil
}

type AllPosition struct {
	Code string `json:"code"`
	Data []struct {
		MarginCoin        string `json:"marginCoin"`
		Symbol            string `json:"symbol"`
		HoldSide          string `json:"holdSide"`
		OpenDelegateCount string `json:"openDelegateCount"`
		Margin            string `json:"margin"`
		AutoMargin        string `json:"autoMargin"`
		Available         string `json:"available"`
		Locked            string `json:"locked"`
		Total             string `json:"total"`
		Leverage          int    `json:"leverage"`
		AchievedProfits   string `json:"achievedProfits"`
		AverageOpenPrice  string `json:"averageOpenPrice"`
		MarginMode        string `json:"marginMode"`
		HoldMode          string `json:"holdMode"`
		UnrealizedPL      string `json:"unrealizedPL"`
		LiquidationPrice  string `json:"liquidationPrice"`
		KeepMarginRate    string `json:"keepMarginRate"`
		MarketPrice       string `json:"marketPrice"`
		CTime             string `json:"cTime"`
		UTime             string `json:"uTime"`
	} `json:"data"`
	Msg         string `json:"msg"`
	RequestTime int64  `json:"requestTime"`
}

func (t *BitgetTrader) GetPositions() ([]map[string]interface{}, error) {
	params := make(map[string]string)
	params["productType"] = "umcbl"

	resp, err := t.client.Get("/api/mix/v1/position/allPosition-v2", params)
	if err != nil {
		log.Println("获取账户信息失败:", err)
		return nil, err
	}
	data := AllPosition{}
	err = json.Unmarshal([]byte(resp), &data)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}

	for _, datum := range data.Data {
		posMap := make(map[string]interface{})
		posMap["symbol"] = datum.Symbol
		posMap["positionAmt"], _ = strconv.ParseFloat(datum.Total, 64)
		posMap["entryPrice"], _ = strconv.ParseFloat(datum.AverageOpenPrice, 64)
		posMap["markPrice"], _ = strconv.ParseFloat(datum.MarketPrice, 64)
		posMap["unRealizedProfit"], _ = strconv.ParseFloat(datum.UnrealizedPL, 64)
		posMap["leverage"] = cast.ToFloat64(datum.Leverage)
		posMap["liquidationPrice"], _ = strconv.ParseFloat(datum.LiquidationPrice, 64)

		// 判断方向
		posMap["side"] = "long"

		result = append(result, posMap)
	}
	return result, nil
}

func (t *BitgetTrader) SetLeverage(symbol string, leverage int) error {
	return nil
}

func (t *BitgetTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return nil, nil
}

func (t *BitgetTrader) OpenLongWithStopAndProfile(symbol string, quantity float64, stopLess, takeProfile string) (map[string]interface{}, error) {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "open_long"
	params["orderType"] = "market"
	params["size"] = cast.ToString(quantity)
	params["timInForceValue"] = "normal"
	params["presetStopLossPrice"] = stopLess

	_, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return nil, nil
}

func (t *BitgetTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	return nil, nil
}

func (t *BitgetTrader) OpenShortWithStopAndProfile(symbol string, quantity float64, stopLess, takeProfile string) (map[string]interface{}, error) {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "open_short"
	params["orderType"] = "market"
	params["size"] = cast.ToString(quantity)
	params["timInForceValue"] = "normal"
	params["presetStopLossPrice"] = stopLess

	_, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return nil, nil
}

func (t *BitgetTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "close_long"
	params["orderType"] = "market"
	params["size"] = cast.ToString(quantity)
	params["timInForceValue"] = "normal"

	_, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return nil, nil
}

func (t *BitgetTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "close_short"
	params["orderType"] = "market"
	params["size"] = cast.ToString(quantity)
	params["timInForceValue"] = "normal"

	_, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return nil, nil
}

func (t *BitgetTrader) CancelAllOrders(symbol string) error {
	return nil
}

func (t *BitgetTrader) GetMarketPrice(symbol string) (float64, error) {
	return 0, nil
}

func (t *BitgetTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	return nil
}

func (t *BitgetTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	return nil
}

func (t *BitgetTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	return "", nil
}
