package polymarket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
)

const (
	GammaAPIEndpoint      = "https://gamma-api.polymarket.com"
	DataAPIEndpoint       = "https://data-api.polymarket.com"
	CLOBEndpoint          = "https://clob.polymarket.com"
	CLOBWebSocketEndpoint = "wss://ws-subscriptions-clob.polymarket.com"
	RTDSWebSocketEndpoint = "wss://ws-live-data.polymarket.com"
)

// NewClient 创建 PolyMarket 客户端
func NewClient(authInfo AuthInfo) *Client {
	return &Client{
		CommonClient: NewCommonClient(authInfo),
	}
}

// Client PolyMarket 客户端
type Client struct {
	*CommonClient
}

var _ ClientInterface = (*Client)(nil)

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

// CreateOrDeriveAPIKey 创建或派生 API 密钥
//
// 如果指定 nonce 还未生成密钥则创建密钥，若已生成则获取已有密钥
func (c *Client) CreateOrDeriveAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error) {
	info, err := c.CreateAPIKey(ctx, nonce)
	if err != nil {
		if errors.Is(err, ErrNonceAlreadyUsed) {
			return c.DeriveAPIKey(ctx, nonce)
		}
		return nil, err
	}
	return info, nil
}

// CreateAPIKey 创建 API 密钥
func (c *Client) CreateAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error) {
	info := &APIKeyInfo{}
	err := c.Do(ctx, &RawRequest{
		Method:     http.MethodPost,
		Endpoint:   CLOBEndpoint,
		URI:        "/auth/api-key",
		WithL1Auth: true,
		L1Nonce:    nonce,
	}, info)
	if err != nil {
		return nil, err
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(1).Info(fmt.Sprintf(
		"created api key: apiKey: %s, secret: %s, passphrase: %s",
		info.APIKey, info.Secret, info.Passphrase,
	))
	return info, nil
}

// DeriveAPIKey 获取已有 API 密钥
func (c *Client) DeriveAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error) {
	info := &APIKeyInfo{}
	err := c.Do(ctx, &RawRequest{
		Method:     http.MethodGet,
		Endpoint:   CLOBEndpoint,
		URI:        "/auth/derive-api-key",
		WithL1Auth: true,
		L1Nonce:    nonce,
	}, info)
	if err != nil {
		return nil, err
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(1).Info(fmt.Sprintf(
		"derived api key: apiKey: %s, secret: %s, passphrase: %s",
		info.APIKey, info.Secret, info.Passphrase,
	))
	return info, nil
}

// SendHeartbeat 发送心跳
func (c *Client) SendHeartbeat(ctx context.Context) (*HeartbeatStatus, error) {
	status := &HeartbeatStatus{}
	err := c.Do(ctx, &RawRequest{
		Method:     http.MethodPost,
		Endpoint:   CLOBEndpoint,
		URI:        "/heartbeats",
		WithL2Auth: true,
	}, status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

// GetUserOrders 获取用户订单
func (c *Client) GetUserOrders(ctx context.Context, req *GetUserOrdersRequest) (*OrdersList, error) {
	query := url.Values{}
	if req.ID != "" {
		query.Set("id", req.ID)
	}
	if req.Maker != "" {
		query.Set("maker", req.Maker)
	}
	if req.AssetID != "" {
		query.Set("asset_id", req.AssetID)
	}
	if req.NextCursor != "" {
		query.Set("next_cursor", req.NextCursor)
	}

	orders := &OrdersList{}
	err := c.Do(ctx, &RawRequest{
		Method:     http.MethodGet,
		Endpoint:   CLOBEndpoint,
		URI:        "/orders",
		Query:      query,
		WithL2Auth: true,
	}, orders)
	if err != nil {
		return nil, err
	}
	return orders, nil
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

// ListEvents 列出事件
func (c *Client) ListEvents(ctx context.Context, req *ListEventsRequest) ([]Event, error) {
	query := url.Values{}
	if req.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", req.Limit))
	}
	if req.Offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", req.Offset))
	}
	if req.Order != "" {
		query.Set("order", req.Order)
	}
	if req.Ascending {
		query.Set("ascending", "true")
	}
	if req.Active != nil {
		query.Set("active", fmt.Sprintf("%t", *req.Active))
	}
	if req.Featured != nil {
		query.Set("featured", fmt.Sprintf("%t", *req.Featured))
	}
	if req.Closed != nil {
		query.Set("closed", fmt.Sprintf("%t", *req.Closed))
	}
	for _, slug := range req.Slug {
		query.Add("slug", slug)
	}

	var events []Event
	err := c.Do(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: GammaAPIEndpoint,
		URI:      "/events",
		Query:    query,
	}, &events)
	if err != nil {
		return nil, err
	}
	return events, nil
}

// ListSeries 列出系列
func (c *Client) ListSeries(ctx context.Context, req *ListSeriesRequest) ([]Series, error) {
	query := url.Values{}
	if req.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", req.Limit))
	}
	if req.Offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", req.Offset))
	}
	if req.Order != "" {
		query.Set("order", req.Order)
	}
	if req.Ascending {
		query.Set("ascending", "true")
	}
	if req.Closed != nil {
		query.Set("closed", fmt.Sprintf("%t", *req.Closed))
	}
	for _, slug := range req.Slug {
		query.Add("slug", slug)
	}

	var series []Series
	err := c.Do(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: GammaAPIEndpoint,
		URI:      "/series",
		Query:    query,
	}, &series)
	if err != nil {
		return nil, err
	}
	return series, nil
}

// Search 搜索事件、系列和用户
func (c *Client) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	query := url.Values{}
	query.Set("q", req.Query)
	if req.Limit > 0 {
		query.Set("limit_per_type", fmt.Sprintf("%d", req.Limit))
	}
	if req.SearchTags {
		query.Set("search_tags", "true")
	}

	result := &SearchResult{}
	err := c.Do(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: GammaAPIEndpoint,
		URI:      "/public-search",
		Query:    query,
	}, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ConnectMarketChannel WebSocket 连接市场信道
func (c *Client) ConnectMarketChannel(ctx context.Context) (*websocket.Conn, error) {
	return c.ConnectWebSocket(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: CLOBWebSocketEndpoint,
		URI:      "/ws/market",
	})
}

// ConnectRTDS WebSocket 连接 RTDS 实时数据服务
func (c *Client) ConnectRTDS(ctx context.Context) (*websocket.Conn, error) {
	return c.ConnectWebSocket(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: RTDSWebSocketEndpoint,
	})
}
