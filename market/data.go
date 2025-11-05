package market

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/duke-git/lancet/v2/convertor"
	"github.com/duke-git/lancet/v2/mathutil"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/spf13/cast"
)

// Data 市场数据结构
type Data struct {
	Symbol            string
	CurrentPrice      float64
	PriceChange1h     float64 // 1小时价格变化百分比
	PriceChange4h     float64 // 4小时价格变化百分比
	CurrentEMA12      float64
	CurrentMACD       float64
	CurrentRSI6       float64
	OpenInterest      *OIData
	OISli             []float64
	FundingRate       float64
	IntradaySeries    *IntradayData
	LongerTermContext *LongerTermData
	MiddleTermContext *LongerTermData
}

type DataV2 struct {
	Symbol            string
	CurrentPrice      float64
	PriceChange1h     float64 // 1小时价格变化百分比
	PriceChange4h     float64 // 4小时价格变化百分比
	CurrentEMA12      float64
	CurrentMACD       float64
	CurrentRSI6       float64
	IntradaySeries    *IntradayData
	LongerTermContext *LongerTermData
	MiddleTermContext *LongerTermData
}

type KlinePlus struct {
	ClosePrice  float64  `json:"close_price"`
	RSI6Value   float64  `json:"rsi6"`
	SMA200Value float64  `json:"sma_200"`
	MACDValue   float64  `json:"macd"`
	BollValue   BollBand `json:"boll"`
}

type BollBand struct {
	BollUpValue   float64 `json:"boll_up"`
	BollDownValue float64 `json:"boll_down"`
	BollMidValue  float64 `json:"boll_mid"`
}

// OIData Open Interest数据
type OIData struct {
	Latest  float64
	Average float64
}

// IntradayData 日内数据(15分钟间隔)
type IntradayData struct {
	MidPrices   []float64
	EMA20Values []float64
	MACDValues  []float64
	RSI6Values  []float64
	RSI14Values []float64
}

// LongerTermData 长期数据(4小时时间框架)
type LongerTermData struct {
	EMA20         float64
	EMA50         float64
	ATR3          float64
	ATR14         float64
	CurrentVolume float64
	AverageVolume float64
	MACDValues    []float64
	RSI14Values   []float64
	FVGValues     []FVG
	KlineValues   []KlinePlus
}

// Kline K线数据
type Kline struct {
	OpenTime  int64   `json:"-"`
	Open      float64 `json:"-"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	CloseTime int64   `json:"-"`
}

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	// 标准化symbol
	symbol = Normalize(symbol)

	// 获取15分钟K线数据 (最近10个)
	klines15m, err := getKlines(symbol, "15m", 24) // 多获取一些用于计算
	if err != nil {
		return nil, fmt.Errorf("获取15分钟K线失败: %v", err)
	}

	// 获取1小时K线数据 (最近10个)
	klines1h, err := getKlines(symbol, "1h", 300) // 多获取一些用于计算
	if err != nil {
		return nil, fmt.Errorf("获取1小时K线失败: %v", err)
	}

	// 获取4小时K线数据 (最近10个)
	klines4h, err := getKlines(symbol, "4h", 300) // 多获取用于计算指标
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	// 计算当前指标 (基于1小时最新数据)
	currentPrice := klines1h[len(klines1h)-1].Close
	currentEMA12 := calculateEMA(klines1h, 12)
	currentMACD := calculateMACD(klines1h)
	currentRSI6 := calculateRSI(klines1h, 6)

	// 计算价格变化百分比
	// 1小时价格变化 = 1个1小时K线前的价格
	priceChange1h := 0.0
	if len(klines1h) >= 2 {
		price4hAgo := klines1h[len(klines1h)-2].Close
		if price4hAgo > 0 {
			priceChange1h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 4小时价格变化 = 1个4小时K线前的价格
	priceChange4h := 0.0
	if len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			priceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 获取OI数据
	oiData, err := getOpenInterestHistData(symbol)
	if err != nil {
		// OI失败不影响整体,使用默认值
		oiData = nil
	}

	//// 获取Funding Rate
	fundingRate, _ := getFundingRate(symbol)

	// 计算日内系列数据
	intradayData := calculateIntradaySeries(klines15m)

	// 计算长期数据
	middleTermData := calculateLongerTermData(klines1h)
	middleTermData.FVGValues = identifyValidFVG(klines1h[:len(klines1h)-1])
	longerTermData := calculateLongerTermData(klines4h)
	longerTermData.FVGValues = identifyValidFVG(klines4h[:len(klines4h)-1])

	return &Data{
		Symbol:        symbol,
		CurrentPrice:  currentPrice,
		PriceChange1h: priceChange1h,
		PriceChange4h: priceChange4h,
		CurrentEMA12:  currentEMA12,
		CurrentMACD:   currentMACD,
		CurrentRSI6:   currentRSI6,
		//OpenInterest:  oiData,
		FundingRate:       fundingRate,
		IntradaySeries:    intradayData,
		OISli:             oiData,
		LongerTermContext: longerTermData,
		MiddleTermContext: middleTermData,
	}, nil
}

// getKlines 从Binance获取K线数据
func getKlines(symbol, interval string, limit int) ([]Kline, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/klines?symbol=%s&interval=%s&limit=%d",
		symbol, interval, limit)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rawData [][]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, err
	}

	klines := make([]Kline, len(rawData))
	for i, item := range rawData {
		openTime := int64(item[0].(float64))
		open, _ := parseFloat(item[1])
		high, _ := parseFloat(item[2])
		low, _ := parseFloat(item[3])
		close, _ := parseFloat(item[4])
		volume, _ := parseFloat(item[5])
		closeTime := int64(item[6].(float64))

		klines[i] = Kline{
			OpenTime:  openTime,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			CloseTime: closeTime,
		}
	}

	return klines, nil
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	// 计算SMA作为初始EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// 计算EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return roundNumber(ema)
}

// calculateMACD 计算MACD
func calculateMACD(klines []Kline) float64 {
	if len(klines) < 26 {
		return 0
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return roundNumber(ema12 - ema26)
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	gains := 0.0
	losses := 0.0

	// 计算初始平均涨跌幅
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// 使用Wilder平滑方法计算后续RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return roundNumber(rsi)
}

// calculateATR 计算ATR
func calculateATR(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// 计算初始ATR
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder平滑
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return roundNumber(atr)
}

// calculateIntradaySeries 计算日内系列数据
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:   make([]float64, 0, 20),
		EMA20Values: make([]float64, 0, 20),
		MACDValues:  make([]float64, 0, 20),
		RSI6Values:  make([]float64, 0, 20),
		RSI14Values: make([]float64, 0, 20),
	}

	// 获取最近10个数据点
	start := len(klines) - 20
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)

		// 计算每个点的EMA20
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// 计算每个点的MACD
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// 计算每个点的RSI
		if i >= 7 {
			rsi6 := calculateRSI(klines[:i+1], 6)
			data.RSI6Values = append(data.RSI6Values, rsi6)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// calculateLongerTermData 计算长期数据
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
		KlineValues: make([]KlinePlus, len(klines), len(klines)),
	}

	// 计算EMA
	//data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)
	//
	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算MACD和RSI序列
	start := len(klines) - 100
	if start < 0 {
		start = 0
	}

	closeSli := make([]float64, 0, 10)
	for _, kline := range klines {
		closeSli = append(closeSli, kline.Close)
	}
	upSli, midSli, lowSli := calculateBollingerBands(closeSli, 21, 2)
	smaSli := calculateSMA(closeSli, 200)
	for i := range data.KlineValues {
		data.KlineValues[i].ClosePrice = klines[i].Close
		boll := BollBand{
			BollUpValue:   upSli[i],
			BollDownValue: midSli[i],
			BollMidValue:  lowSli[i],
		}
		data.KlineValues[i].BollValue = boll
		data.KlineValues[i].SMA200Value = smaSli[i]
	}
	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
			data.KlineValues[i].MACDValue = macd
		}
		if i >= 6 {
			rsi6 := calculateRSI(klines[:i+1], 6)
			data.KlineValues[i].RSI6Value = rsi6
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}
	data.KlineValues = data.KlineValues[len(data.KlineValues)-24:]

	return data
}

func calculateBollingerBands(prices []float64, n int, k float64) (upperBand, middleBand, lowerBand []float64) {
	if len(prices) < n {
		return nil, nil, nil
	}

	var sma []float64
	var stdDev []float64

	for i := n - 1; i < len(prices); i++ {
		sum := 0.0
		for j := i - n + 1; j <= i; j++ {
			sum += prices[j]
		}
		sma = append(sma, sum/float64(n))

		var varianceSum float64
		for j := i - n + 1; j <= i; j++ {
			varianceSum += math.Pow(prices[j]-sma[len(sma)-1], 2)
		}
		stdDev = append(stdDev, math.Sqrt(varianceSum/float64(n)))

		upperBand = append(upperBand, roundNumber(sma[len(sma)-1]+k*stdDev[len(stdDev)-1]))
		middleBand = append(middleBand, roundNumber(sma[len(sma)-1]))
		lowerBand = append(lowerBand, roundNumber(sma[len(sma)-1]-k*stdDev[len(stdDev)-1]))
	}

	if len(upperBand) < len(prices) {
		temp := make([]float64, len(prices)-len(upperBand), len(prices)-len(upperBand))
		temp = append(temp, upperBand...)
		upperBand = temp
	}

	if len(middleBand) < len(prices) {
		temp := make([]float64, len(prices)-len(middleBand), len(prices)-len(middleBand))
		temp = append(temp, middleBand...)
		middleBand = temp
	}

	if len(lowerBand) < len(prices) {
		temp := make([]float64, len(prices)-len(lowerBand), len(prices)-len(lowerBand))
		temp = append(temp, lowerBand...)
		lowerBand = temp
	}

	return upperBand, middleBand, lowerBand
}

func calculateSMA(prices []float64, n int) []float64 {
	if len(prices) < n {
		return nil
	}

	var sma []float64

	for i := n - 1; i < len(prices); i++ {
		sum := 0.0
		for j := i - n + 1; j <= i; j++ {
			sum += prices[j]
		}
		sma = append(sma, roundNumber(sum/float64(n)))
	}
	if len(sma) < len(prices) {
		temp := make([]float64, len(prices)-len(sma), len(prices)-len(sma))
		temp = append(temp, sma...)
		sma = temp
	}
	return sma
}

// getOpenInterestData 获取OI数据
func getOpenInterestData(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)

	return &OIData{
		Latest:  oi,
		Average: oi * 0.999, // 近似平均值
	}, nil
}

// getOpenInterestHistData 获取持仓数据
func getOpenInterestHistData(symbol string) ([]float64, error) {
	url := fmt.Sprintf("https://fapi.binance.com/futures/data/openInterestHist?symbol=%s&period=5m", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result []struct {
		Symbol               string `json:"symbol"`
		SumOpenInterest      string `json:"sumOpenInterest"`
		SumOpenInterestValue string `json:"sumOpenInterestValue"`
		Timestamp            int64  `json:"timestamp"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oiSli := make([]float64, 0)
	for _, s := range result {
		sv := cast.ToFloat64(s.SumOpenInterestValue)
		if sv > 0 {
			oiSli = append(oiSli, sv)
		}
	}
	return oiSli, nil
}

// getFundingRate 获取资金费率
func getFundingRate(symbol string) (float64, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/premiumIndex?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		Symbol          string `json:"symbol"`
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
		InterestRate    string `json:"interestRate"`
		Time            int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	rate, _ := strconv.ParseFloat(result.LastFundingRate, 64)
	return rate, nil
}

// Format 格式化输出市场数据
func Format(data *Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("1‑hour timeframe: current_price = %.2f, current_ema12 = %.2f\n\n",
		data.CurrentPrice, data.CurrentEMA12))

	if len(data.OISli) > 0 {
		sb.WriteString(fmt.Sprintf("open interest history indicators (5‑minute intervals, oldest → latest): %s\n\n", formatFloatSlice(data.OISli)))
	}

	//sb.WriteString(fmt.Sprintf("In addition, here is the latest %s open interest and funding rate for perps:\n\n",
	//	data.Symbol))
	//
	//if data.OpenInterest != nil {
	//	sb.WriteString(fmt.Sprintf("Open Interest: Latest: %.2f Average: %.2f\n\n",
	//		data.OpenInterest.Latest, data.OpenInterest.Average))
	//}
	//
	//sb.WriteString(fmt.Sprintf("Funding Rate: %.2e\n\n", data.FundingRate))

	if data.IntradaySeries != nil {
		sb.WriteString("Intraday series (15‑minute intervals, oldest → latest):\n\n")

		if len(data.IntradaySeries.MidPrices) > 0 {
			sb.WriteString(fmt.Sprintf("Mid prices: %s\n\n", formatFloatSlice(data.IntradaySeries.MidPrices)))
		}

		if len(data.IntradaySeries.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA indicators (20‑period): %s\n\n", formatFloatSlice(data.IntradaySeries.EMA20Values)))
		}

		if len(data.IntradaySeries.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.IntradaySeries.MACDValues)))
		}

		if len(data.IntradaySeries.RSI6Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (6‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI6Values)))
		}

		if len(data.IntradaySeries.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.IntradaySeries.RSI14Values)))
		}
	}

	if data.MiddleTermContext != nil {
		sb.WriteString("Longer‑term context (1‑hour timeframe):\n\n")

		sb.WriteString(fmt.Sprintf("50‑Period EMA: %.3f\n\n",
			data.LongerTermContext.EMA50))
		//
		sb.WriteString(fmt.Sprintf("3‑Period ATR: %.3f vs. 14‑Period ATR: %.3f\n\n",
			data.MiddleTermContext.ATR3, data.LongerTermContext.ATR14))

		sb.WriteString(fmt.Sprintf("Current Volume: %.3f vs. Average Volume: %.3f\n\n",
			data.MiddleTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))

		//if len(data.MiddleTermContext.MACDValues) > 0 {
		//	sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
		//}
		//
		//if len(data.MiddleTermContext.RSI14Values) > 0 {
		//	sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
		//}

		if len(data.MiddleTermContext.FVGValues) > 0 {
			sb.WriteString(fmt.Sprintf("FVG(Fair Value Gap) json data: %s\n\n", convertor.ToString(data.MiddleTermContext.FVGValues)))
		}
		if len(data.MiddleTermContext.KlineValues) > 0 {
			sb.WriteString(fmt.Sprintf("technology json data: %s\n\n", convertor.ToString(data.MiddleTermContext.KlineValues)))
		}
	}

	if data.LongerTermContext != nil {
		sb.WriteString("Longer‑term context (4‑hour timeframe):\n\n")

		sb.WriteString(fmt.Sprintf("50‑Period EMA: %.3f\n\n",
			data.LongerTermContext.EMA50))
		//
		sb.WriteString(fmt.Sprintf("3‑Period ATR: %.3f vs. 14‑Period ATR: %.3f\n\n",
			data.LongerTermContext.ATR3, data.LongerTermContext.ATR14))

		sb.WriteString(fmt.Sprintf("Current Volume: %.3f vs. Average Volume: %.3f\n\n",
			data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))

		//if len(data.LongerTermContext.MACDValues) > 0 {
		//	sb.WriteString(fmt.Sprintf("MACD indicators: %s\n\n", formatFloatSlice(data.LongerTermContext.MACDValues)))
		//}
		//
		//if len(data.LongerTermContext.RSI14Values) > 0 {
		//	sb.WriteString(fmt.Sprintf("RSI indicators (14‑Period): %s\n\n", formatFloatSlice(data.LongerTermContext.RSI14Values)))
		//}
		if len(data.LongerTermContext.FVGValues) > 0 {
			sb.WriteString(fmt.Sprintf("FVG(Fair Value Gap) json data: %s\n\n", convertor.ToString(data.LongerTermContext.FVGValues)))
		}
		if len(data.LongerTermContext.KlineValues) > 0 {
			sb.WriteString(fmt.Sprintf("technology json data: %s\n\n", convertor.ToString(data.LongerTermContext.KlineValues)))
		}
	}

	return sb.String()
}

// formatFloatSlice 格式化float64切片为字符串
func formatFloatSlice(values []float64) string {
	strValues := make([]string, len(values))
	for i, v := range values {
		strValues[i] = fmt.Sprintf("%.3f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}

// Normalize 标准化symbol,确保是USDT交易对
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// parseFloat 解析float值
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}

// 定义 FVG 结构
type FVG struct {
	High    float64   // FVG （最高点）
	Low     float64   // FVG （最低点）
	Time    time.Time // FVG 中间K线开盘时间
	IsValid bool      // 标记 FVG 是否有效
	Type    string    // FVG 类型："1上涨" 或 "2下跌"
}

// 将时间戳转换为可读时间
func timestampToTime(ts int64) time.Time {
	return time.UnixMilli(ts)
}

func identifyValidFVG(kLines []Kline) []FVG {
	var fvgList []FVG

	// 遍历所有 K 线，按规则识别 FVG
	for i := 2; i < len(kLines); i++ {
		k1 := kLines[i-2] // 第1根 K 线
		k2 := kLines[i-1] // 第2根 K 线
		k3 := kLines[i]   // 第3根 K 线

		// 判断是否形成上涨 FVG（价格向上突破）
		if k1.High < k3.Low && k2.Close > k1.High && k2.Close > k3.Low && k3.Close > k1.Close {
			// 创建上涨 FVG
			fvg := FVG{
				High:    k3.Low,
				Low:     k1.High,
				Time:    timestampToTime(k2.OpenTime),
				IsValid: true,
				Type:    "上涨",
			}
			fvgList = append(fvgList, fvg)
		}

		// 判断是否形成下跌 FVG（价格向下突破）
		if k1.Low < k3.High && k2.Close < k1.Low && k2.Close < k3.High && k3.Close < k1.Close {
			// 创建下跌 FVG
			fvg := FVG{
				High:    k1.Low,
				Low:     k3.High,
				Time:    timestampToTime(k2.OpenTime),
				IsValid: true,
				Type:    "下跌",
			}
			fvgList = append(fvgList, fvg)
		}

		// 检查是否有 FVG 被后续 K 线实体穿过
		for j := range fvgList {
			// 如果 FVG 已经无效，则跳过
			if !fvgList[j].IsValid {
				continue
			}
			if fvgList[j].Type == "上涨" && k3.Close < fvgList[j].Low { //实体跌到下边界外
				fvgList[j].IsValid = false // 标记为无效
			}

			if fvgList[j].Type == "下跌" && k3.Close > fvgList[j].High { //实体冲到上边界外
				fvgList[j].IsValid = false // 标记为无效
			}
		}
	}
	fvgList = slice.Filter(fvgList, func(index int, item FVG) bool {
		return item.IsValid
	})

	return fvgList
}

func roundNumber(num float64) float64 {
	if num == 0 {
		return num
	}
	absNum := math.Abs(num)
	if absNum > 10 {
		return mathutil.RoundToFloat(num, 2)
	} else if absNum > 1 {
		return mathutil.RoundToFloat(num, 4)
	} else {
		temp := absNum
		loop := 0
		for temp < 1 {
			temp *= 10
			loop++
		}
		return mathutil.RoundToFloat(num, loop+3)
	}
}
