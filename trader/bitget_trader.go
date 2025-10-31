package trader

import (
	"bitget/config"
	"bitget/pkg/client"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/duke-git/lancet/v2/mathutil"
	"github.com/spf13/cast"
)

type BitgetTrader struct {
	client *client.BitgetApiClient

	// 余额缓存
	cachedBalance     map[string]interface{}
	balanceCacheTime  time.Time
	balanceCacheMutex sync.RWMutex

	// 缓存有效期（15秒）
	cacheDuration time.Duration
}

func NewBitgetTrader(apiKey, secretKey string) *BitgetTrader {
	config.Init(apiKey, secretKey)
	clt := new(client.BitgetApiClient).Init()
	return &BitgetTrader{client: clt}
}

type Accounts struct {
	Code string `json:"code"`
	Data []struct {
		MarginCoin        string      `json:"marginCoin"`
		Locked            string      `json:"locked"`
		Available         string      `json:"available"`
		CrossMaxAvailable string      `json:"crossMaxAvailable"`
		FixedMaxAvailable string      `json:"fixedMaxAvailable"`
		MaxTransferOut    string      `json:"maxTransferOut"`
		Equity            string      `json:"equity"`
		UsdtEquity        string      `json:"usdtEquity"`
		BtcEquity         string      `json:"btcEquity"`
		CrossRiskRate     string      `json:"crossRiskRate"`
		UnrealizedPL      interface{} `json:"unrealizedPL"`
		Bonus             string      `json:"bonus"`
	} `json:"data"`
	Msg         string `json:"msg"`
	RequestTime int64  `json:"requestTime"`
}

type Account struct {
	MarginCoin        string      `json:"marginCoin"`
	Locked            string      `json:"locked"`
	Available         string      `json:"available"`
	CrossMaxAvailable string      `json:"crossMaxAvailable"`
	FixedMaxAvailable string      `json:"fixedMaxAvailable"`
	MaxTransferOut    string      `json:"maxTransferOut"`
	Equity            string      `json:"equity"`
	UsdtEquity        string      `json:"usdtEquity"`
	BtcEquity         string      `json:"btcEquity"`
	CrossRiskRate     string      `json:"crossRiskRate"`
	UnrealizedPL      interface{} `json:"unrealizedPL"`
	Bonus             string      `json:"bonus"`
}

func (t *BitgetTrader) GetBalance() (map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.balanceCacheTime)
		t.balanceCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的账户余额（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedBalance, nil
	}
	t.balanceCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用BitgetAPI获取账户余额...")
	params := make(map[string]string)
	params["productType"] = "umcbl"
	params["marginCoin"] = "USDT"

	resp, err := t.client.Get("/api/mix/v1/account/accounts", params)
	if err != nil {
		log.Println("获取账户信息失败:", err)
		return nil, err
	}
	accounts := Accounts{}
	err = json.Unmarshal([]byte(resp), &accounts)
	if err != nil {
		log.Println("解析账户数据失败:", err)
		log.Println("BGAPI返回:", resp)
		return nil, err
	}
	if len(accounts.Data) == 0 {
		log.Println("获取账户数据为空:", err)
		return nil, err
	}
	account := Account{}
	for i, datum := range accounts.Data {
		if datum.MarginCoin == "USDT" {
			account = accounts.Data[i]
		}
	}

	result := make(map[string]interface{})
	result["totalWalletBalance"] = cast.ToFloat64(account.UsdtEquity) - cast.ToFloat64(account.UnrealizedPL)
	result["availableBalance"] = cast.ToFloat64(account.Available)
	result["totalUnrealizedProfit"] = cast.ToFloat64(account.UnrealizedPL)

	log.Printf("✓ BGAPI返回: 总余额=%s, 可用=%s, 未实现盈亏=%s",
		cast.ToString(account.UsdtEquity),
		cast.ToString(account.Available),
		cast.ToString(account.UnrealizedPL))

	// 更新缓存
	t.balanceCacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return result, nil
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
		log.Println("获取持仓信息失败:", err)
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
		posMap["symbol"] = strings.TrimSuffix(datum.Symbol, "_UMCBL")
		posMap["positionAmt"], _ = strconv.ParseFloat(datum.Available, 64)
		posMap["entryPrice"], _ = strconv.ParseFloat(datum.AverageOpenPrice, 64)
		posMap["markPrice"], _ = strconv.ParseFloat(datum.MarketPrice, 64)
		posMap["unRealizedProfit"], _ = strconv.ParseFloat(datum.UnrealizedPL, 64)
		posMap["leverage"] = cast.ToFloat64(datum.Leverage)
		posMap["liquidationPrice"], _ = strconv.ParseFloat(datum.LiquidationPrice, 64)
		posMap["side"] = datum.HoldSide

		result = append(result, posMap)
	}
	return result, nil
}

func (t *BitgetTrader) SetLeverage(symbol string, leverage int) error {
	return nil
}

type OrderResp struct {
	Code string `json:"code"`
	Data struct {
		OrderID   string `json:"orderId"`
		ClientOid string `json:"clientOid"`
	} `json:"data"`
	Msg         string `json:"msg"`
	RequestTime int64  `json:"requestTime"`
}

func (t *BitgetTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "open_long"
	params["orderType"] = "market"
	params["size"] = quantityStr
	params["timInForceValue"] = "normal"

	resp, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	order := OrderResp{}
	err = json.Unmarshal([]byte(resp), &order)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	if order.Code != "00000" {
		log.Println("  ❌ 创建订单失败:", order.Msg)
	}
	result := map[string]interface{}{
		"symbol":  symbol,
		"orderId": order.Data.OrderID,
		"status":  order.Code,
	}
	return result, nil
}

func (t *BitgetTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "open_short"
	params["orderType"] = "market"
	params["size"] = quantityStr
	params["timInForceValue"] = "normal"

	resp, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	order := OrderResp{}
	err = json.Unmarshal([]byte(resp), &order)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	if order.Code != "00000" {
		log.Println("  ❌ 创建订单失败:", order.Msg)
	}
	result := map[string]interface{}{
		"symbol":  symbol,
		"orderId": order.Data.OrderID,
		"status":  order.Code,
	}
	return result, nil
}

func (t *BitgetTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if cast.ToString(pos["symbol"]) == symbol {
				quantity = math.Abs(cast.ToFloat64(pos["positionAmt"])) // 空仓数量是负的，取绝对值
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的多仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "close_long"
	params["orderType"] = "market"
	params["size"] = quantityStr
	params["timInForceValue"] = "normal"

	resp, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	order := OrderResp{}
	err = json.Unmarshal([]byte(resp), &order)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	if order.Code != "00000" {
		log.Println("  ❌ 创建订单失败:", order.Msg)
	}
	result := map[string]interface{}{
		"symbol":  symbol,
		"orderId": order.Data.OrderID,
		"status":  order.Code,
	}
	return result, nil
}

func (t *BitgetTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if cast.ToString(pos["symbol"]) == symbol {
				quantity = math.Abs(cast.ToFloat64(pos["positionAmt"])) // 空仓数量是负的，取绝对值
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的空仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["side"] = "close_short"
	params["orderType"] = "market"
	params["size"] = quantityStr
	params["timInForceValue"] = "normal"

	resp, err := t.client.Post("/api/mix/v1/order/placeOrder", params)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	order := OrderResp{}
	err = json.Unmarshal([]byte(resp), &order)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	if order.Code != "00000" {
		log.Println("  ❌ 创建订单失败:", order.Msg)
	}
	result := map[string]interface{}{
		"symbol":  symbol,
		"orderId": order.Data.OrderID,
		"status":  order.Code,
	}
	return result, nil
}

func (t *BitgetTrader) CancelAllOrders(symbol string) error {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"

	_, err := t.client.Get("/api/mix/v1/order/cancel-symbol-orders", params)
	if err != nil {
		log.Println("撤单失败:", err)
		return err
	}
	return nil
}

type OneTicker struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Symbol             string `json:"symbol"`
		Last               string `json:"last"`
		BestAsk            string `json:"bestAsk"`
		BestBid            string `json:"bestBid"`
		BidSz              string `json:"bidSz"`
		AskSz              string `json:"askSz"`
		High24H            string `json:"high24h"`
		Low24H             string `json:"low24h"`
		Timestamp          string `json:"timestamp"`
		PriceChangePercent string `json:"priceChangePercent"`
		BaseVolume         string `json:"baseVolume"`
		QuoteVolume        string `json:"quoteVolume"`
		UsdtVolume         string `json:"usdtVolume"`
		OpenUtc            string `json:"openUtc"`
		ChgUtc             string `json:"chgUtc"`
		IndexPrice         string `json:"indexPrice"`
		FundingRate        string `json:"fundingRate"`
		HoldingAmount      string `json:"holdingAmount"`
	} `json:"data"`
}

func (t *BitgetTrader) GetMarketPrice(symbol string) (float64, error) {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"

	resp, err := t.client.Get("/api/mix/v1/market/ticker", params)
	if err != nil {
		log.Println("获取当前价格失败:", err)
		return 0, err
	}
	tinker := OneTicker{}
	err = json.Unmarshal([]byte(resp), &tinker)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	return cast.ToFloat64(tinker.Data.Last), nil
}

func (t *BitgetTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["holdSide"] = positionSide
	params["planType"] = "pos_loss"
	params["triggerPrice"] = cast.ToString(stopPrice)
	params["triggerType"] = "market_price"

	resp, err := t.client.Post("/api/mix/v1/plan/placePositionsTPSL", params)
	if err != nil {
		log.Println(err)
		return err
	}
	order := OrderResp{}
	err = json.Unmarshal([]byte(resp), &order)
	if err != nil {
		log.Println(err)
		return err
	}
	if order.Code != "00000" {
		log.Println("  ❌ 创建止损失败:", order.Msg)
	}
	return nil
}

func (t *BitgetTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	params := make(map[string]string)
	params["symbol"] = symbol + "_UMCBL"
	params["marginCoin"] = "USDT"
	params["holdSide"] = positionSide
	params["planType"] = "pos_profit"
	params["triggerPrice"] = cast.ToString(takeProfitPrice)
	params["triggerType"] = "market_price"

	resp, err := t.client.Post("/api/mix/v1/plan/placePositionsTPSL", params)
	if err != nil {
		log.Println(err)
		return err
	}
	order := OrderResp{}
	err = json.Unmarshal([]byte(resp), &order)
	if err != nil {
		log.Println(err)
		return err
	}
	if order.Code != "00000" {
		log.Println("  ❌ 创建止盈失败:", order.Msg)
	}
	return nil
}

type BitgetContracts struct {
	Code string `json:"code"`
	Data []struct {
		BaseCoin             string   `json:"baseCoin"`
		BaseCoinDisplayName  string   `json:"baseCoinDisplayName"`
		BuyLimitPriceRatio   string   `json:"buyLimitPriceRatio"`
		FeeRateUpRatio       string   `json:"feeRateUpRatio"`
		MakerFeeRate         string   `json:"makerFeeRate "`
		MinTradeNum          string   `json:"minTradeNum"`
		PriceEndStep         string   `json:"priceEndStep"`
		PricePlace           string   `json:"pricePlace " `
		QuoteCoin            string   `json:""`
		QuoteCoinDisplayName string   `json:"quoteCoinDisplayName"`
		SellLimitPriceRatio  string   `json:"sellLimitPriceRatio"`
		SizeMultiplier       string   `json:"sizeMultiplier"`
		SupportMarginCoins   []string `json:"supportMarginCoins"`
		Symbol               string   `json:"symbol"`
		SymbolDisplayName    string   `json:"symbolDisplayName"`
		TakerFeeRate         string   `json:"takerFeeRate"`
		VolumePlace          string   `json:"volumePlace"`
		SymbolType           string   `json:"symbolType"`
		SymbolStatus         string   `json:"symbolStatus"`
		OffTime              string   `json:"offTime"`
		LimitOpenTime        string   `json:"limitOpenTime"`
	} `json:"data"`
	Msg         string `json:"msg"`
	RequestTime int64  `json:"requestTime"`
}

// GetSymbolPrecision 获取交易对的数量精度
func (t *BitgetTrader) GetSymbolPrecision(symbol string) (int, error) {
	params := make(map[string]string)
	params["productType"] = "umcbl"

	resp, err := t.client.Get("/api/mix/v1/market/contracts", params)
	if err != nil {
		log.Println("获取交易对信息失败:", err)
		return 3, err
	}
	data := BitgetContracts{}
	err = json.Unmarshal([]byte(resp), &data)
	if err != nil {
		return 3, err
	}
	for _, datum := range data.Data {
		if strings.TrimSuffix(datum.Symbol, "_UMCBL") == symbol {
			stepSize := cast.ToString(datum.MinTradeNum)
			precision := calculatePrecision(stepSize)
			log.Printf("  %s 数量精度: %d (stepSize: %s)", symbol, precision, stepSize)
			return precision, nil
		}
	}
	log.Printf("  ⚠ %s 未找到精度信息，使用默认精度3", symbol)
	return 3, nil // 默认精度为3
}

func (t *BitgetTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	precision, _ := t.GetSymbolPrecision(symbol)
	quantity = mathutil.RoundToFloat(quantity, precision)
	return cast.ToString(quantity), nil
}
