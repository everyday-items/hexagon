// Package api 提供常用 API 集成工具
//
// 本包实现多种常用 API 工具：
//   - Weather: 天气查询
//   - Stock: 股票数据
//   - News: 新闻获取
//   - Currency: 货币汇率
//   - Translation: 文本翻译
//
// 设计借鉴：
//   - LangChain: Tool 体系
//   - Semantic Kernel: Plugin 系统
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/everyday-items/ai-core/schema"
	"github.com/everyday-items/ai-core/tool"
)

// ============== 天气 API ==============

// WeatherTool 天气查询工具
type WeatherTool struct {
	// APIKey API 密钥
	APIKey string

	// Provider 天气服务提供商
	Provider WeatherProvider

	// HTTPClient HTTP 客户端
	HTTPClient *http.Client
}

// WeatherProvider 天气服务提供商
type WeatherProvider string

const (
	// WeatherProviderOpenWeather OpenWeatherMap
	WeatherProviderOpenWeather WeatherProvider = "openweathermap"

	// WeatherProviderWeatherAPI WeatherAPI.com
	WeatherProviderWeatherAPI WeatherProvider = "weatherapi"

	// WeatherProviderQWeather 和风天气
	WeatherProviderQWeather WeatherProvider = "qweather"
)

// WeatherInput 天气查询输入
type WeatherInput struct {
	// Location 位置（城市名或坐标）
	Location string `json:"location" desc:"城市名称或经纬度坐标" required:"true"`

	// Units 温度单位：metric（摄氏度）或 imperial（华氏度）
	Units string `json:"units,omitempty" desc:"温度单位" enum:"metric,imperial" default:"metric"`

	// Lang 语言
	Lang string `json:"lang,omitempty" desc:"返回语言" default:"zh_cn"`
}

// WeatherOutput 天气查询输出
type WeatherOutput struct {
	// Location 位置
	Location string `json:"location"`

	// Temperature 温度
	Temperature float64 `json:"temperature"`

	// FeelsLike 体感温度
	FeelsLike float64 `json:"feels_like"`

	// Humidity 湿度（百分比）
	Humidity int `json:"humidity"`

	// Description 天气描述
	Description string `json:"description"`

	// WindSpeed 风速
	WindSpeed float64 `json:"wind_speed"`

	// WindDirection 风向
	WindDirection string `json:"wind_direction,omitempty"`

	// Pressure 气压
	Pressure int `json:"pressure,omitempty"`

	// Visibility 能见度（米）
	Visibility int `json:"visibility,omitempty"`

	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// NewWeatherTool 创建天气工具
func NewWeatherTool(apiKey string, provider WeatherProvider) *WeatherTool {
	return &WeatherTool{
		APIKey:     apiKey,
		Provider:   provider,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name 工具名称
func (t *WeatherTool) Name() string {
	return "weather"
}

// Description 工具描述
func (t *WeatherTool) Description() string {
	return "查询指定城市或位置的实时天气信息，包括温度、湿度、天气状况等"
}

// Schema 输入 Schema
func (t *WeatherTool) Schema() *schema.Schema {
	return schema.Of[WeatherInput]()
}

// Execute 执行查询
func (t *WeatherTool) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	input := &WeatherInput{}
	data, _ := json.Marshal(args)
	if err := json.Unmarshal(data, input); err != nil {
		return tool.Result{}, fmt.Errorf("invalid input: %w", err)
	}

	if input.Units == "" {
		input.Units = "metric"
	}
	if input.Lang == "" {
		input.Lang = "zh_cn"
	}

	var output *WeatherOutput
	var err error

	switch t.Provider {
	case WeatherProviderOpenWeather:
		output, err = t.fetchOpenWeather(ctx, input)
	case WeatherProviderWeatherAPI:
		output, err = t.fetchWeatherAPI(ctx, input)
	case WeatherProviderQWeather:
		output, err = t.fetchQWeather(ctx, input)
	default:
		output, err = t.fetchOpenWeather(ctx, input)
	}

	if err != nil {
		return tool.Result{Error: err.Error()}, nil
	}

	return tool.Result{Success: true, Output: output}, nil
}

// Validate 验证参数
func (t *WeatherTool) Validate(args map[string]any) error {
	if _, ok := args["location"]; !ok {
		return fmt.Errorf("location is required")
	}
	return nil
}

func (t *WeatherTool) fetchOpenWeather(ctx context.Context, input *WeatherInput) (*WeatherOutput, error) {
	u := fmt.Sprintf(
		"https://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=%s&lang=%s",
		url.QueryEscape(input.Location),
		t.APIKey,
		input.Units,
		input.Lang,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var data struct {
		Name    string `json:"name"`
		Main    struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			Humidity  int     `json:"humidity"`
			Pressure  int     `json:"pressure"`
		} `json:"main"`
		Weather []struct {
			Description string `json:"description"`
		} `json:"weather"`
		Wind struct {
			Speed float64 `json:"speed"`
			Deg   int     `json:"deg"`
		} `json:"wind"`
		Visibility int   `json:"visibility"`
		Dt         int64 `json:"dt"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	output := &WeatherOutput{
		Location:      data.Name,
		Temperature:   data.Main.Temp,
		FeelsLike:     data.Main.FeelsLike,
		Humidity:      data.Main.Humidity,
		Pressure:      data.Main.Pressure,
		WindSpeed:     data.Wind.Speed,
		WindDirection: degreeToDirection(data.Wind.Deg),
		Visibility:    data.Visibility,
		UpdatedAt:     time.Unix(data.Dt, 0),
	}

	if len(data.Weather) > 0 {
		output.Description = data.Weather[0].Description
	}

	return output, nil
}

func (t *WeatherTool) fetchWeatherAPI(ctx context.Context, input *WeatherInput) (*WeatherOutput, error) {
	u := fmt.Sprintf(
		"https://api.weatherapi.com/v1/current.json?key=%s&q=%s&lang=%s",
		t.APIKey,
		url.QueryEscape(input.Location),
		input.Lang,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var data struct {
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
		Current struct {
			TempC      float64 `json:"temp_c"`
			TempF      float64 `json:"temp_f"`
			FeelsLikeC float64 `json:"feelslike_c"`
			FeelsLikeF float64 `json:"feelslike_f"`
			Humidity   int     `json:"humidity"`
			Condition  struct {
				Text string `json:"text"`
			} `json:"condition"`
			WindKph    float64 `json:"wind_kph"`
			WindDir    string  `json:"wind_dir"`
			PressureMb float64 `json:"pressure_mb"`
			VisKm      float64 `json:"vis_km"`
		} `json:"current"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	temp := data.Current.TempC
	feelsLike := data.Current.FeelsLikeC
	if input.Units == "imperial" {
		temp = data.Current.TempF
		feelsLike = data.Current.FeelsLikeF
	}

	return &WeatherOutput{
		Location:      data.Location.Name,
		Temperature:   temp,
		FeelsLike:     feelsLike,
		Humidity:      data.Current.Humidity,
		Description:   data.Current.Condition.Text,
		WindSpeed:     data.Current.WindKph / 3.6, // 转换为 m/s
		WindDirection: data.Current.WindDir,
		Pressure:      int(data.Current.PressureMb),
		Visibility:    int(data.Current.VisKm * 1000),
		UpdatedAt:     time.Now(),
	}, nil
}

func (t *WeatherTool) fetchQWeather(ctx context.Context, input *WeatherInput) (*WeatherOutput, error) {
	// 和风天气实现
	u := fmt.Sprintf(
		"https://devapi.qweather.com/v7/weather/now?location=%s&key=%s&lang=%s",
		url.QueryEscape(input.Location),
		t.APIKey,
		input.Lang,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		Code string `json:"code"`
		Now  struct {
			Temp      string `json:"temp"`
			FeelsLike string `json:"feelsLike"`
			Humidity  string `json:"humidity"`
			Text      string `json:"text"`
			WindSpeed string `json:"windSpeed"`
			WindDir   string `json:"windDir"`
			Pressure  string `json:"pressure"`
			Vis       string `json:"vis"`
		} `json:"now"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if data.Code != "200" {
		return nil, fmt.Errorf("API error: code %s", data.Code)
	}

	var temp, feelsLike, windSpeed float64
	var humidity, pressure, vis int
	fmt.Sscanf(data.Now.Temp, "%f", &temp)
	fmt.Sscanf(data.Now.FeelsLike, "%f", &feelsLike)
	fmt.Sscanf(data.Now.Humidity, "%d", &humidity)
	fmt.Sscanf(data.Now.WindSpeed, "%f", &windSpeed)
	fmt.Sscanf(data.Now.Pressure, "%d", &pressure)
	fmt.Sscanf(data.Now.Vis, "%d", &vis)

	return &WeatherOutput{
		Location:      input.Location,
		Temperature:   temp,
		FeelsLike:     feelsLike,
		Humidity:      humidity,
		Description:   data.Now.Text,
		WindSpeed:     windSpeed / 3.6,
		WindDirection: data.Now.WindDir,
		Pressure:      pressure,
		Visibility:    vis * 1000,
		UpdatedAt:     time.Now(),
	}, nil
}

func degreeToDirection(deg int) string {
	directions := []string{"北", "东北", "东", "东南", "南", "西南", "西", "西北"}
	idx := ((deg + 22) / 45) % 8
	return directions[idx]
}

// ============== 股票 API ==============

// StockTool 股票数据工具
type StockTool struct {
	APIKey     string
	Provider   StockProvider
	HTTPClient *http.Client
}

// StockProvider 股票数据提供商
type StockProvider string

const (
	// StockProviderAlphaVantage Alpha Vantage
	StockProviderAlphaVantage StockProvider = "alphavantage"

	// StockProviderEastMoney 东方财富
	StockProviderEastMoney StockProvider = "eastmoney"
)

// StockInput 股票查询输入
type StockInput struct {
	// Symbol 股票代码
	Symbol string `json:"symbol" desc:"股票代码，如 AAPL, 600519.SH" required:"true"`

	// Type 查询类型：quote（实时报价）, history（历史数据）
	Type string `json:"type,omitempty" desc:"查询类型" enum:"quote,history" default:"quote"`
}

// StockQuote 股票实时报价
type StockQuote struct {
	Symbol        string    `json:"symbol"`
	Name          string    `json:"name,omitempty"`
	Price         float64   `json:"price"`
	Change        float64   `json:"change"`
	ChangePercent float64   `json:"change_percent"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Volume        int64     `json:"volume"`
	MarketCap     float64   `json:"market_cap,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NewStockTool 创建股票工具
func NewStockTool(apiKey string, provider StockProvider) *StockTool {
	return &StockTool{
		APIKey:     apiKey,
		Provider:   provider,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name 工具名称
func (t *StockTool) Name() string {
	return "stock"
}

// Description 工具描述
func (t *StockTool) Description() string {
	return "查询股票实时行情，包括价格、涨跌、成交量等信息"
}

// Schema 输入 Schema
func (t *StockTool) Schema() *schema.Schema {
	return schema.Of[StockInput]()
}

// Execute 执行查询
func (t *StockTool) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	input := &StockInput{}
	data, _ := json.Marshal(args)
	if err := json.Unmarshal(data, input); err != nil {
		return tool.Result{}, fmt.Errorf("invalid input: %w", err)
	}

	quote, err := t.getQuote(ctx, input.Symbol)
	if err != nil {
		return tool.Result{Error: err.Error()}, nil
	}

	return tool.Result{Success: true, Output: quote}, nil
}

// Validate 验证参数
func (t *StockTool) Validate(args map[string]any) error {
	if _, ok := args["symbol"]; !ok {
		return fmt.Errorf("symbol is required")
	}
	return nil
}

func (t *StockTool) getQuote(ctx context.Context, symbol string) (*StockQuote, error) {
	u := fmt.Sprintf(
		"https://www.alphavantage.co/query?function=GLOBAL_QUOTE&symbol=%s&apikey=%s",
		url.QueryEscape(symbol),
		t.APIKey,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		GlobalQuote struct {
			Symbol        string `json:"01. symbol"`
			Open          string `json:"02. open"`
			High          string `json:"03. high"`
			Low           string `json:"04. low"`
			Price         string `json:"05. price"`
			Volume        string `json:"06. volume"`
			LatestDay     string `json:"07. latest trading day"`
			PrevClose     string `json:"08. previous close"`
			Change        string `json:"09. change"`
			ChangePercent string `json:"10. change percent"`
		} `json:"Global Quote"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	var price, open, high, low, change, changePercent float64
	var volume int64

	fmt.Sscanf(data.GlobalQuote.Price, "%f", &price)
	fmt.Sscanf(data.GlobalQuote.Open, "%f", &open)
	fmt.Sscanf(data.GlobalQuote.High, "%f", &high)
	fmt.Sscanf(data.GlobalQuote.Low, "%f", &low)
	fmt.Sscanf(data.GlobalQuote.Change, "%f", &change)
	changePercentStr := strings.TrimSuffix(data.GlobalQuote.ChangePercent, "%")
	fmt.Sscanf(changePercentStr, "%f", &changePercent)
	fmt.Sscanf(data.GlobalQuote.Volume, "%d", &volume)

	return &StockQuote{
		Symbol:        data.GlobalQuote.Symbol,
		Price:         price,
		Open:          open,
		High:          high,
		Low:           low,
		Change:        change,
		ChangePercent: changePercent,
		Volume:        volume,
		UpdatedAt:     time.Now(),
	}, nil
}

// ============== 新闻 API ==============

// NewsTool 新闻获取工具
type NewsTool struct {
	APIKey     string
	Provider   NewsProvider
	HTTPClient *http.Client
}

// NewsProvider 新闻提供商
type NewsProvider string

const (
	// NewsProviderNewsAPI NewsAPI.org
	NewsProviderNewsAPI NewsProvider = "newsapi"

	// NewsProviderGNews GNews
	NewsProviderGNews NewsProvider = "gnews"
)

// NewsInput 新闻查询输入
type NewsInput struct {
	// Query 搜索关键词
	Query string `json:"query,omitempty" desc:"搜索关键词"`

	// Category 分类
	Category string `json:"category,omitempty" desc:"新闻分类" enum:"business,entertainment,general,health,science,sports,technology"`

	// Country 国家代码
	Country string `json:"country,omitempty" desc:"国家代码，如 cn, us" default:"cn"`

	// Language 语言
	Language string `json:"language,omitempty" desc:"语言代码" default:"zh"`

	// PageSize 返回数量
	PageSize int `json:"page_size,omitempty" desc:"返回新闻数量" default:"10"`
}

// NewsArticle 新闻文章
type NewsArticle struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Content     string    `json:"content,omitempty"`
	URL         string    `json:"url"`
	ImageURL    string    `json:"image_url,omitempty"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
}

// NewsOutput 新闻输出
type NewsOutput struct {
	TotalResults int            `json:"total_results"`
	Articles     []NewsArticle  `json:"articles"`
}

// NewNewsTool 创建新闻工具
func NewNewsTool(apiKey string, provider NewsProvider) *NewsTool {
	return &NewsTool{
		APIKey:     apiKey,
		Provider:   provider,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name 工具名称
func (t *NewsTool) Name() string {
	return "news"
}

// Description 工具描述
func (t *NewsTool) Description() string {
	return "获取最新新闻资讯，支持按关键词搜索和分类筛选"
}

// Schema 输入 Schema
func (t *NewsTool) Schema() *schema.Schema {
	return schema.Of[NewsInput]()
}

// Execute 执行查询
func (t *NewsTool) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	input := &NewsInput{}
	data, _ := json.Marshal(args)
	if err := json.Unmarshal(data, input); err != nil {
		return tool.Result{}, fmt.Errorf("invalid input: %w", err)
	}

	if input.PageSize <= 0 {
		input.PageSize = 10
	}

	output, err := t.fetchNews(ctx, input)
	if err != nil {
		return tool.Result{Error: err.Error()}, nil
	}

	return tool.Result{Success: true, Output: output}, nil
}

// Validate 验证参数
func (t *NewsTool) Validate(args map[string]any) error {
	return nil
}

func (t *NewsTool) fetchNews(ctx context.Context, input *NewsInput) (*NewsOutput, error) {
	var u string
	if input.Query != "" {
		u = fmt.Sprintf(
			"https://newsapi.org/v2/everything?q=%s&language=%s&pageSize=%d&apiKey=%s",
			url.QueryEscape(input.Query),
			input.Language,
			input.PageSize,
			t.APIKey,
		)
	} else {
		u = fmt.Sprintf(
			"https://newsapi.org/v2/top-headlines?country=%s&pageSize=%d&apiKey=%s",
			input.Country,
			input.PageSize,
			t.APIKey,
		)
		if input.Category != "" {
			u += "&category=" + input.Category
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		Status       string `json:"status"`
		TotalResults int    `json:"totalResults"`
		Articles     []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Content     string `json:"content"`
			URL         string `json:"url"`
			URLToImage  string `json:"urlToImage"`
			Source      struct {
				Name string `json:"name"`
			} `json:"source"`
			PublishedAt time.Time `json:"publishedAt"`
		} `json:"articles"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if data.Status != "ok" {
		return nil, fmt.Errorf("API error: %s", data.Status)
	}

	articles := make([]NewsArticle, len(data.Articles))
	for i, a := range data.Articles {
		articles[i] = NewsArticle{
			Title:       a.Title,
			Description: a.Description,
			Content:     a.Content,
			URL:         a.URL,
			ImageURL:    a.URLToImage,
			Source:      a.Source.Name,
			PublishedAt: a.PublishedAt,
		}
	}

	return &NewsOutput{
		TotalResults: data.TotalResults,
		Articles:     articles,
	}, nil
}

// ============== 货币汇率 API ==============

// CurrencyTool 货币汇率工具
type CurrencyTool struct {
	APIKey     string
	HTTPClient *http.Client
}

// CurrencyInput 汇率查询输入
type CurrencyInput struct {
	// From 源货币代码
	From string `json:"from" desc:"源货币代码，如 USD, CNY" required:"true"`

	// To 目标货币代码
	To string `json:"to" desc:"目标货币代码" required:"true"`

	// Amount 金额
	Amount float64 `json:"amount,omitempty" desc:"转换金额" default:"1"`
}

// CurrencyOutput 汇率输出
type CurrencyOutput struct {
	From       string    `json:"from"`
	To         string    `json:"to"`
	Rate       float64   `json:"rate"`
	Amount     float64   `json:"amount"`
	Converted  float64   `json:"converted"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NewCurrencyTool 创建货币工具
func NewCurrencyTool(apiKey string) *CurrencyTool {
	return &CurrencyTool{
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name 工具名称
func (t *CurrencyTool) Name() string {
	return "currency"
}

// Description 工具描述
func (t *CurrencyTool) Description() string {
	return "查询货币汇率并进行货币转换"
}

// Schema 输入 Schema
func (t *CurrencyTool) Schema() *schema.Schema {
	return schema.Of[CurrencyInput]()
}

// Execute 执行查询
func (t *CurrencyTool) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	input := &CurrencyInput{}
	data, _ := json.Marshal(args)
	if err := json.Unmarshal(data, input); err != nil {
		return tool.Result{}, fmt.Errorf("invalid input: %w", err)
	}

	if input.Amount <= 0 {
		input.Amount = 1
	}

	output, err := t.convert(ctx, input)
	if err != nil {
		return tool.Result{Error: err.Error()}, nil
	}

	return tool.Result{Success: true, Output: output}, nil
}

// Validate 验证参数
func (t *CurrencyTool) Validate(args map[string]any) error {
	if _, ok := args["from"]; !ok {
		return fmt.Errorf("from is required")
	}
	if _, ok := args["to"]; !ok {
		return fmt.Errorf("to is required")
	}
	return nil
}

func (t *CurrencyTool) convert(ctx context.Context, input *CurrencyInput) (*CurrencyOutput, error) {
	u := fmt.Sprintf(
		"https://api.exchangerate-api.com/v4/latest/%s",
		strings.ToUpper(input.From),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		Base  string             `json:"base"`
		Rates map[string]float64 `json:"rates"`
		Date  string             `json:"date"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	rate, ok := data.Rates[strings.ToUpper(input.To)]
	if !ok {
		return nil, fmt.Errorf("unsupported currency: %s", input.To)
	}

	return &CurrencyOutput{
		From:      input.From,
		To:        input.To,
		Rate:      rate,
		Amount:    input.Amount,
		Converted: input.Amount * rate,
		UpdatedAt: time.Now(),
	}, nil
}

// ============== 翻译 API ==============

// TranslationTool 翻译工具
type TranslationTool struct {
	APIKey     string
	Provider   TranslationProvider
	HTTPClient *http.Client
}

// TranslationProvider 翻译服务提供商
type TranslationProvider string

const (
	// TranslationProviderDeepL DeepL
	TranslationProviderDeepL TranslationProvider = "deepl"

	// TranslationProviderGoogle Google Translate
	TranslationProviderGoogle TranslationProvider = "google"

	// TranslationProviderBaidu 百度翻译
	TranslationProviderBaidu TranslationProvider = "baidu"
)

// TranslationInput 翻译输入
type TranslationInput struct {
	// Text 待翻译文本
	Text string `json:"text" desc:"待翻译的文本" required:"true"`

	// SourceLang 源语言（可选，自动检测）
	SourceLang string `json:"source_lang,omitempty" desc:"源语言代码，如 en, zh, ja"`

	// TargetLang 目标语言
	TargetLang string `json:"target_lang" desc:"目标语言代码" required:"true"`
}

// TranslationOutput 翻译输出
type TranslationOutput struct {
	OriginalText   string `json:"original_text"`
	TranslatedText string `json:"translated_text"`
	SourceLang     string `json:"source_lang"`
	TargetLang     string `json:"target_lang"`
	Confidence     float64 `json:"confidence,omitempty"`
}

// NewTranslationTool 创建翻译工具
func NewTranslationTool(apiKey string, provider TranslationProvider) *TranslationTool {
	return &TranslationTool{
		APIKey:     apiKey,
		Provider:   provider,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 工具名称
func (t *TranslationTool) Name() string {
	return "translate"
}

// Description 工具描述
func (t *TranslationTool) Description() string {
	return "将文本从一种语言翻译成另一种语言"
}

// Schema 输入 Schema
func (t *TranslationTool) Schema() *schema.Schema {
	return schema.Of[TranslationInput]()
}

// Execute 执行翻译
func (t *TranslationTool) Execute(ctx context.Context, args map[string]any) (tool.Result, error) {
	input := &TranslationInput{}
	data, _ := json.Marshal(args)
	if err := json.Unmarshal(data, input); err != nil {
		return tool.Result{}, fmt.Errorf("invalid input: %w", err)
	}

	output, err := t.translate(ctx, input)
	if err != nil {
		return tool.Result{Error: err.Error()}, nil
	}

	return tool.Result{Success: true, Output: output}, nil
}

// Validate 验证参数
func (t *TranslationTool) Validate(args map[string]any) error {
	if _, ok := args["text"]; !ok {
		return fmt.Errorf("text is required")
	}
	if _, ok := args["target_lang"]; !ok {
		return fmt.Errorf("target_lang is required")
	}
	return nil
}

func (t *TranslationTool) translate(ctx context.Context, input *TranslationInput) (*TranslationOutput, error) {
	switch t.Provider {
	case TranslationProviderDeepL:
		return t.translateDeepL(ctx, input)
	default:
		return t.translateDeepL(ctx, input)
	}
}

func (t *TranslationTool) translateDeepL(ctx context.Context, input *TranslationInput) (*TranslationOutput, error) {
	u := "https://api-free.deepl.com/v2/translate"

	form := url.Values{}
	form.Set("text", input.Text)
	form.Set("target_lang", strings.ToUpper(input.TargetLang))
	if input.SourceLang != "" {
		form.Set("source_lang", strings.ToUpper(input.SourceLang))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+t.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var data struct {
		Translations []struct {
			DetectedSourceLanguage string `json:"detected_source_language"`
			Text                   string `json:"text"`
		} `json:"translations"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if len(data.Translations) == 0 {
		return nil, fmt.Errorf("no translation result")
	}

	sourceLang := input.SourceLang
	if sourceLang == "" {
		sourceLang = data.Translations[0].DetectedSourceLanguage
	}

	return &TranslationOutput{
		OriginalText:   input.Text,
		TranslatedText: data.Translations[0].Text,
		SourceLang:     sourceLang,
		TargetLang:     input.TargetLang,
	}, nil
}

// ============== 工具注册 ==============

// RegisterAPITools 注册所有 API 工具
func RegisterAPITools(registry interface {
	Register(tool.Tool) error
}, configs map[string]string) error {
	if apiKey, ok := configs["weather_api_key"]; ok {
		if err := registry.Register(NewWeatherTool(apiKey, WeatherProviderOpenWeather)); err != nil {
			return err
		}
	}

	if apiKey, ok := configs["stock_api_key"]; ok {
		if err := registry.Register(NewStockTool(apiKey, StockProviderAlphaVantage)); err != nil {
			return err
		}
	}

	if apiKey, ok := configs["news_api_key"]; ok {
		if err := registry.Register(NewNewsTool(apiKey, NewsProviderNewsAPI)); err != nil {
			return err
		}
	}

	if apiKey, ok := configs["currency_api_key"]; ok {
		if err := registry.Register(NewCurrencyTool(apiKey)); err != nil {
			return err
		}
	}

	if apiKey, ok := configs["translation_api_key"]; ok {
		if err := registry.Register(NewTranslationTool(apiKey, TranslationProviderDeepL)); err != nil {
			return err
		}
	}

	return nil
}
