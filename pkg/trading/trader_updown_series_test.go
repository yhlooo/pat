package trading

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestUpdown5m15m 测试 Updown5m15m
func TestUpdown5m15m(t *testing.T) {
	a := assert.New(t)

	a.Equal("btc-updown-5m", BTCUpdown5m.Slug())
	a.Equal("btc-updown-15m", BTCUpdown15m.Slug())
	a.Equal("eth-updown-5m", ETHUpdown5m.Slug())

	t1 := time.Unix(1774970123, 0)
	a.Equal("btc-updown-5m-1774970100", BTCUpdown5m.ActiveMarketSlugForTime(t1))
	a.Equal("btc-updown-15m-1774970100", BTCUpdown15m.ActiveMarketSlugForTime(t1))
	a.Equal("eth-updown-5m-1774970100", ETHUpdown5m.ActiveMarketSlugForTime(t1))

	t2 := time.Unix(1774970100, 0)
	a.Equal("btc-updown-5m-1774970100", BTCUpdown5m.ActiveMarketSlugForTime(t2))
	a.Equal("btc-updown-15m-1774970100", BTCUpdown15m.ActiveMarketSlugForTime(t2))
	a.Equal("eth-updown-5m-1774970100", ETHUpdown5m.ActiveMarketSlugForTime(t2))
}
