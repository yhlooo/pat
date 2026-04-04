package polymarket

const (
	GammaAPIEndpoint      = "https://gamma-api.polymarket.com"
	DataAPIEndpoint       = "https://data-api.polymarket.com"
	CLOBEndpoint          = "https://clob.polymarket.com"
	CLOBWebSocketEndpoint = "wss://ws-subscriptions-clob.polymarket.com"
	RTDSWebSocketEndpoint = "wss://ws-live-data.polymarket.com"
)

// ClientInterface PolyMarket 客户端
type ClientInterface interface {
	CommonClientInterface
	GammaAPIClient
	DataAPIClient
	CLOBReaderClient
	CLOBWriterClient
	RTDSClient
}

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
