package polymarket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// GammaAPIClient Gamma API 客户端
type GammaAPIClient interface {
	// GetEventBySlug 通过 slug 获取事件
	GetEventBySlug(ctx context.Context, req *GetEventBySlugRequest) (*Event, error)
	// GetMarketBySlug 通过 slug 获取市场
	GetMarketBySlug(ctx context.Context, slug string) (*Market, error)
}

// GetEventBySlug 通过 slug 获取事件
func (c *Client) GetEventBySlug(ctx context.Context, req *GetEventBySlugRequest) (*Event, error) {
	query := url.Values{}
	if req.IncludeChat {
		query.Set("include_chat", "true")
	}
	if req.IncludeTemplate {
		query.Set("include_template", "true")
	}

	event := &Event{}
	err := c.Do(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: GammaAPIEndpoint,
		URI:      "/events/slug/" + req.Slug,
		Query:    query,
	}, event)
	if err != nil {
		return nil, err
	}
	return event, nil
}

// GetMarketBySlug 通过 slug 获取市场
func (c *Client) GetMarketBySlug(ctx context.Context, slug string) (*Market, error) {
	market := &Market{}
	err := c.Do(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: GammaAPIEndpoint,
		URI:      "/markets/slug/" + slug,
	}, market)
	if err != nil {
		return nil, err
	}
	return market, nil
}

// GetEventBySlugRequest 通过 slug 获取事件请求
type GetEventBySlugRequest struct {
	Slug            string
	IncludeChat     bool
	IncludeTemplate bool
}

// Event 事件
type Event struct {
	ID               string   `json:"id"`
	Ticker           string   `json:"ticker,omitempty"`
	Slug             string   `json:"slug,omitempty"`
	Title            string   `json:"title,omitempty"`
	SubTitle         string   `json:"subtitle,omitempty"`
	Description      string   `json:"description,omitempty"`
	StartDate        string   `json:"startDate,omitempty"`
	EndDate          string   `json:"endDate,omitempty"`
	Image            string   `json:"image,omitempty"`
	Icon             string   `json:"icon,omitempty"`
	Active           bool     `json:"active,omitempty"`
	Closed           bool     `json:"closed,omitempty"`
	Featured         bool     `json:"featured,omitempty"`
	Volume           float64  `json:"volume,omitempty"`
	Volume24hr       float64  `json:"volume24hr,omitempty"`
	Liquidity        float64  `json:"liquidity,omitempty"`
	OpenInterest     float64  `json:"openInterest,omitempty"`
	Category         string   `json:"category,omitempty"`
	SeriesSlug       string   `json:"seriesSlug,omitempty"`
	ResolutionSource string   `json:"resolutionSource,omitempty"`
	Markets          []Market `json:"markets,omitempty"`
	Series           []Series `json:"series,omitempty"`
}

// Series 系列
type Series struct {
	ID          string  `json:"id"`
	Ticker      string  `json:"ticker,omitempty"`
	Slug        string  `json:"slug,omitempty"`
	Title       string  `json:"title,omitempty"`
	SubTitle    string  `json:"subtitle,omitempty"`
	Description string  `json:"description,omitempty"`
	Image       string  `json:"image,omitempty"`
	Icon        string  `json:"icon,omitempty"`
	Active      bool    `json:"active,omitempty"`
	Closed      bool    `json:"closed,omitempty"`
	Featured    bool    `json:"featured,omitempty"`
	Volume      float64 `json:"volume,omitempty"`
	Volume24hr  float64 `json:"volume24hr,omitempty"`
	Liquidity   float64 `json:"liquidity,omitempty"`
	Events      []Event `json:"events,omitempty"`
}

// Market 市场信息
type Market struct {
	ID            string            `json:"id"`
	Question      string            `json:"question"`
	Description   string            `json:"description"`
	ConditionID   string            `json:"conditionId"`
	Slug          string            `json:"slug"`
	EndDate       time.Time         `json:"endDate"`
	CLOBTokenIDs  string            `json:"clobTokenIds"`  // JSON 字符串数组
	Outcomes      string            `json:"outcomes"`      // JSON 字符串数组，例如 ["Yes","No"]
	OutcomePrices string            `json:"outcomePrices"` // JSON 字符串数组，例如 ["0.52","0.48"]
	Volume        float64           `json:"volumeNum,omitempty"`
	Volume24hr    float64           `json:"volume24hr,omitempty"`
	Liquidity     float64           `json:"liquidityNum,omitempty"`
	BestBid       float64           `json:"bestBid,omitempty"`
	BestAsk       float64           `json:"bestAsk,omitempty"`
	Active        bool              `json:"active,omitempty"`
	Closed        bool              `json:"closed,omitempty"`
	Events        []MarketEventData `json:"events,omitempty"`
}

// GetCLOBTokenIDs 获取 CLOB Token ID 列表
func (m Market) GetCLOBTokenIDs() ([2]string, error) {
	var ret [2]string
	return ret, json.Unmarshal([]byte(m.CLOBTokenIDs), &ret)
}

// GetOutcomes 获取结果名列表
func (m Market) GetOutcomes() ([2]string, error) {
	var ret [2]string
	return ret, json.Unmarshal([]byte(m.Outcomes), &ret)
}

// MarketEventData 市场关联的事件信息
type MarketEventData struct {
	ID               string        `json:"id"`
	Slug             string        `json:"slug,omitempty"`
	ResolutionSource string        `json:"resolutionSource,omitempty"`
	EventMetadata    EventMetadata `json:"eventMetadata,omitempty"`
}

// EventMetadata 事件元数据
type EventMetadata struct {
	PriceToBeat *float64 `json:"priceToBeat"`
	FinalPrice  *float64 `json:"finalPrice,omitempty"`
}
