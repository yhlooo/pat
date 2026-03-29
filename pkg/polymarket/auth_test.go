package polymarket

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthInfo_L1Sign
func TestAuthInfo_L1Sign(t *testing.T) {
	a := assert.New(t)

	auth := AuthInfo{
		PrivateKey: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
	}

	s, err := auth.L1Sign("1774360713", 0)
	a.NoError(err)
	a.Equal("0xf1e06cf7ea211f0e657ce37ffcc83b7413f8e68bc9e96192920a98871776a7680"+
		"6faa6f0c636b88fde97410850eeb6bf10799dd2d5c974d033eef5238de164e41c", s)

	s, err = auth.L1Sign("1774360713", 1)
	a.NoError(err)
	a.Equal("0x26214b9a1933ff51d61a66e011ca06b47b0b38f9acd489d064b267d5d514f51a5"+
		"820252bb8510ebb09dc53aff1c6ff3b5d4a3e4e6e10d0ff27eee1e112eea85a1b", s)

	s, err = auth.L1Sign("1774360713", 2)
	a.NoError(err)
	a.Equal("0x68104d41b1d42e9d03c0374fd407e9aa2d3655f8c8ce8b798250cd1278d8b0db6"+
		"3e4af1c65b2aed310e57f987326d879ce8fe425f17399e6dd3fdd0a6a83242a1b", s)

	s, err = auth.L1Sign("1774360713", 3)
	a.NoError(err)
	a.Equal("0x966b2c66acb10503e3a6cd7233dd65e63188fa61cbf417adc40b6647035a09956"+
		"5bae5ccfe0af4ae9cc57856f2b2a4b4dda629d3175a2c6f348b17d9a75de7a01b", s)

	s, err = auth.L1Sign("1774360713", 4)
	a.NoError(err)
	a.Equal("0xafebdb982f18a852d0b8647ec1ac4e82324c3a0abd40cffe63e34f36fd61a5b82"+
		"83bd92ab98551e56ffe4fc1fe73cb6162fce1105abb9a620c34dcadcdeb476c1c", s)
}

// TestAuthInfo_L2Sign 测试 AuthInfo.L2Sign 方法
func TestAuthInfo_L2Sign(t *testing.T) {
	a := assert.New(t)

	auth := AuthInfo{
		Secret: "0000000000000000000000000000000000000000000=",
	}
	s, err := auth.L2Sign(http.MethodGet, "/test", "1774178860", []byte(`{"key":"value"}`))
	a.NoError(err)
	a.Equal("6wSe9QjNN_grhxOt_20RRBMcVUo0yQKAAa4iFdIvLQc=", s)
}

// TestAuthInfo_SetL2AuthHeader 测试 AuthInfo.SetL2AuthHeader 方法
func TestAuthInfo_SetL2AuthHeader(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// 构造测试请求
	req, err := http.NewRequest(http.MethodGet, "/test?k=v", nil)
	r.NoError(err)

	// 构造测试认证信息
	// 随机生成的认证信息，无任何实际作用
	auth := AuthInfo{
		Address:    "0x1234567890ABCDEF01234567890ABCDEF1234567",
		APIKey:     "12345678-1234-1234-1234-123456789012",
		Secret:     "0000000000000000000000000000000000000000000=",
		Passphrase: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
	}

	r.NoError(auth.SetL2AuthHeader(req, []byte(`{"key":"value"}`)))
	a.Equal("0x1234567890abcdef01234567890abcdef1234567", req.Header.Get("POLY_ADDRESS"))
	a.NotEmpty(req.Header.Get("POLY_SIGNATURE"))
	a.Len(req.Header.Get("POLY_TIMESTAMP"), 10)
	a.Equal("12345678-1234-1234-1234-123456789012", req.Header.Get("POLY_API_KEY"))
	a.Equal("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", req.Header.Get("POLY_PASSPHRASE"))
}

// TestAddressFromPrivateKey 测试 AddressFromPrivateKey
func TestAddressFromPrivateKey(t *testing.T) {
	a := assert.New(t)

	addr, err := AddressFromPrivateKey("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	a.NoError(err)
	a.Equal("0x1be31a94361a391bbafb2a4ccd704f57dc04d4bb", addr)
}
