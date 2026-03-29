package polymarket

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// ClientInterface PolyMarket 客户端
type ClientInterface interface {
	CommonClientInterface
	GammaAPIClient
	DataAPIClient
	CLOBReaderClient
	CLOBWriterClient
}

// GammaAPIClient Gamma API 客户端
type GammaAPIClient interface {
	// GetEventBySlug 通过 slug 获取事件
	GetEventBySlug(ctx context.Context, req *GetEventBySlugRequest) (*Event, error)
	// GetMarketBySlug 通过 slug 获取市场
	GetMarketBySlug(ctx context.Context, slug string) (*Market, error)
	// ListEvents 列出事件
	ListEvents(ctx context.Context, req *ListEventsRequest) ([]Event, error)
	// ListSeries 列出系列
	ListSeries(ctx context.Context, req *ListSeriesRequest) ([]Series, error)
	// Search 搜索事件、系列和用户
	Search(ctx context.Context, req *SearchRequest) (*SearchResult, error)
}

// DataAPIClient Data API 客户端
type DataAPIClient interface{}

// CLOBReaderClient CLOB 读 API 客户端
type CLOBReaderClient interface {
	// ConnectMarketChannel WebSocket 连接市场信道
	ConnectMarketChannel(ctx context.Context) (*websocket.Conn, error)
}

// CLOBWriterClient CLOB 写 API 客户端
type CLOBWriterClient interface {
	// SendHeartbeat 发送心跳
	SendHeartbeat(ctx context.Context) (*HeartbeatStatus, error)
	// GetUserOrders 获取用户订单
	GetUserOrders(ctx context.Context, req *GetUserOrdersRequest) (*OrdersList, error)
}

// CommonClientInterface 通用客户端
type CommonClientInterface interface {
	// AuthInfo 获取认证信息
	AuthInfo() AuthInfo
	// SetAuthInfo 设置认证信息
	SetAuthInfo(authInfo AuthInfo)
}

// APIKeyInfo API 密钥信息
type APIKeyInfo struct {
	APIKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

// ListMeta 列表元信息
type ListMeta struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor"`
	Count      int    `json:"count"`
}

// GetEventBySlugRequest 通过 slug 获取事件请求
type GetEventBySlugRequest struct {
	Slug            string
	IncludeChat     bool
	IncludeTemplate bool
}

// ListEventsRequest 列出事件请求
type ListEventsRequest struct {
	Limit     int
	Offset    int
	Order     string // 例如 "volume24hr", "liquidity"
	Ascending bool
	Active    *bool
	Featured  *bool
	Closed    *bool
	Slug      []string
}

// ListSeriesRequest 列出系列请求
type ListSeriesRequest struct {
	Limit     int
	Offset    int
	Order     string
	Ascending bool
	Closed    *bool
	Slug      []string
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Query      string
	Limit      int
	SearchTags bool
}

// SearchResult 搜索结果
type SearchResult struct {
	Events     []Event     `json:"events"`
	Tags       []SearchTag `json:"tags,omitempty"`
	Profiles   []Profile   `json:"profiles,omitempty"`
	Pagination Pagination  `json:"pagination,omitempty"`
}

// SearchTag 搜索标签
type SearchTag struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Slug       string `json:"slug"`
	EventCount int    `json:"event_count,omitempty"`
}

// Profile 用户资料
type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Pseudonym string `json:"pseudonym,omitempty"`
}

// Pagination 分页信息
type Pagination struct {
	HasMore      bool `json:"hasMore"`
	TotalResults int  `json:"totalResults"`
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

// GetUserOrdersRequest 获取用户订单请求
type GetUserOrdersRequest struct {
	ID         string
	Maker      string
	AssetID    string
	NextCursor string
}

// OrdersList 订单列表
type OrdersList struct {
	ListMeta
	Data []Order `json:"data"`
}

// Order 订单
type Order struct {
	ID              string        `json:"id"`
	Status          string        `json:"status"`
	Owner           string        `json:"owner"`
	MakerAddress    string        `json:"maker_address"`
	Market          string        `json:"market"`
	AssetID         string        `json:"asset_id"`
	Side            string        `json:"side"`
	OriginalSize    string        `json:"original_size"`
	SizeMatched     string        `json:"size_matched"`
	Price           string        `json:"price"`
	Outcome         string        `json:"outcome"`
	Expiration      string        `json:"expiration"`
	OrderType       string        `json:"order_type"`
	CreatedAt       int           `json:"created_at"`
	AssociateTrades []interface{} `json:"associate_trades,omitempty"`
}

// HeartbeatStatus 心跳状态
type HeartbeatStatus struct {
	Status string `json:"status"`
}

// Market 市场信息
type Market struct {
	ID            string            `json:"id"`
	Question      string            `json:"question"`
	Description   string            `json:"description"`
	ConditionID   string            `json:"conditionId"`
	Slug          string            `json:"slug"`
	ClobTokenIDs  string            `json:"clobTokenIds"`  // JSON 字符串数组
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

// ResolutionSourceInfo resolutionSource 解析结果
type ResolutionSourceInfo struct {
	Asset  string // 资产符号，如 "btc"
	Quote  string // 计价货币，如 "usd"
	Topic  string // RTDS 订阅 topic，如 "crypto_prices_chainlink"
	Symbol string // RTDS 订阅 symbol，如 "btc/usd"
}

// ParseResolutionSource 从 markets 的 events 中解析底层资产信息
// 仅支持 Chainlink 格式: https://data.chain.link/streams/{asset}-{quote}
func ParseResolutionSource(market *Market) *ResolutionSourceInfo {
	if market == nil {
		return nil
	}
	for _, e := range market.Events {
		if info := parseChainlinkURL(e.ResolutionSource); info != nil {
			return info
		}
	}
	return nil
}

// parseChainlinkURL 解析 Chainlink URL
func parseChainlinkURL(resolutionSource string) *ResolutionSourceInfo {
	if resolutionSource == "" {
		return nil
	}
	u, err := url.Parse(resolutionSource)
	if err != nil {
		return nil
	}
	// 匹配 data.chain.link/streams/{asset}-{quote}
	if u.Host != "data.chain.link" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/streams/"), "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	asset, quote := parts[0], parts[1]
	return &ResolutionSourceInfo{
		Asset:  asset,
		Quote:  quote,
		Topic:  "crypto_prices_chainlink",
		Symbol: fmt.Sprintf("%s/%s", asset, quote),
	}
}
