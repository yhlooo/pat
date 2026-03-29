package polymarket

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// AuthInfo 认证信息
type AuthInfo struct {
	// 签名者账户地址 ( L2 必须)
	//
	// 比如 MetaMask 钱包账户地址，注意不是 PolyMarket 中的代理钱包地址
	Address string
	// 签名者私钥，与 Address 对应 (L1 认证必须)
	PrivateKey string
	// PolyMarket 用户 API Key (L2 认证必须)
	APIKey string
	// PolyMarket 用户 secret (L2 认证必须)
	Secret string
	// PolyMarket 用户 passphrase (L2 认证必须)
	Passphrase string
	// Relayer API Key (Relayer API 必须)
	RelayerAPIKey string
	// Relayer 地址 (Relayer API 必须)
	RelayerAddress string
}

// GetAddress 获取地址
func (auth AuthInfo) GetAddress() string {
	if auth.PrivateKey == "" {
		return strings.ToLower(auth.Address)
	}
	addr, _ := AddressFromPrivateKey(auth.PrivateKey)
	return addr
}

// HasL1Auth 判断是否具有 L1 认证信息
func (auth AuthInfo) HasL1Auth() bool {
	return auth.PrivateKey != ""
}

// L1Sign 生成 L1 签名 (CLOB EIP-712 签名)
func (auth AuthInfo) L1Sign(ts string, nonce int64) (string, error) {
	if auth.PrivateKey == "" {
		return "", fmt.Errorf("%w: private key missing", ErrInvalidAuthInfo)
	}

	// 解析私钥
	privateKeyHex := strings.TrimPrefix(auth.PrivateKey, "0x")
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}

	// Polygon Chain ID
	chainID := big.NewInt(137)

	// 构建 EIP-712 Domain Separator
	// domain = { name: "ClobAuthDomain", version: "1", chainId: 137 }
	domainSeparatorData := make([]byte, 0, 32*4)
	domainSeparatorData = append(domainSeparatorData, crypto.Keccak256([]byte(
		"EIP712Domain(string name,string version,uint256 chainId)",
	))...)
	domainSeparatorData = append(domainSeparatorData, crypto.Keccak256([]byte("ClobAuthDomain"))...)
	domainSeparatorData = append(domainSeparatorData, crypto.Keccak256([]byte("1"))...)
	domainSeparatorData = append(domainSeparatorData, common.LeftPadBytes(chainID.Bytes(), 32)...)
	domainSeparator := crypto.Keccak256(domainSeparatorData)

	// 构建 ClobAuth 结构哈希
	structData := make([]byte, 0, 32*5)
	structData = append(structData, crypto.Keccak256([]byte(
		"ClobAuth(address address,string timestamp,uint256 nonce,string message)",
	))...)
	structData = append(structData, common.LeftPadBytes(common.HexToAddress(auth.GetAddress()).Bytes(), 32)...)
	structData = append(structData, crypto.Keccak256([]byte(ts))...)
	structData = append(structData, common.LeftPadBytes(binary.BigEndian.AppendUint64(nil, uint64(nonce)), 32)...)
	structData = append(structData, crypto.Keccak256([]byte(
		"This message attests that I control the given wallet",
	))...)
	structHash := crypto.Keccak256(structData)

	// 构建最终签名数据: keccak256("\x19\x01" ‖ domainSeparator ‖ structHash)
	signData := make([]byte, 0, 2+32+32)
	signData = append(signData, []byte("\x19\x01")...)
	signData = append(signData, domainSeparator...)
	signData = append(signData, structHash...)
	signDataHash := crypto.Keccak256Hash(signData)

	// 使用私钥签名
	signature, err := crypto.Sign(signDataHash.Bytes(), privateKey)
	if err != nil {
		return "", fmt.Errorf("sign error: %w", err)
	}

	// 调整 v 值 (EIP-155 兼容签名 v = 27 或 28)
	if signature[64] < 27 {
		signature[64] += 27
	}

	return "0x" + common.Bytes2Hex(signature), nil
}

// SetL1AuthHeader 设置 L1 认证请求头
func (auth AuthInfo) SetL1AuthHeader(req *http.Request, nonce int64) error {
	if !auth.HasL1Auth() {
		return fmt.Errorf("%w: l1 auth info missing", ErrInvalidAuthInfo)
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)

	sign, err := auth.L1Sign(ts, nonce)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSignError, err)
	}

	req.Header.Set("POLY_ADDRESS", auth.GetAddress())
	req.Header.Set("POLY_SIGNATURE", sign)
	req.Header.Set("POLY_TIMESTAMP", ts)
	req.Header.Set("POLY_NONCE", strconv.FormatInt(nonce, 10))

	return nil
}

// WithL2Auth 返回加上 L2 的认证信息
func (auth AuthInfo) WithL2Auth(apiKey, secret, passphrase string) AuthInfo {
	newAuth := auth
	newAuth.Address = auth.GetAddress()
	newAuth.APIKey = apiKey
	newAuth.Secret = secret
	newAuth.Passphrase = passphrase
	return newAuth
}

// HasL2Auth 判断是否具有 L2 认证信息
func (auth AuthInfo) HasL2Auth() bool {
	return auth.Address != "" && auth.APIKey != "" && auth.Secret != "" && auth.Passphrase != ""
}

// L2Sign 生成 L2 签名 (请求的 HMAC 签名)
func (auth AuthInfo) L2Sign(method, uri, ts string, body []byte) (string, error) {
	if auth.Secret == "" {
		return "", fmt.Errorf("%w: secret missing", ErrInvalidAuthInfo)
	}

	secretRaw, err := base64.URLEncoding.DecodeString(auth.Secret)
	if err != nil {
		return "", fmt.Errorf("invalid secret: %w", err)
	}
	h := hmac.New(sha256.New, secretRaw)
	msg := ts + method + uri + string(body)
	h.Write([]byte(msg))
	return base64.URLEncoding.EncodeToString(h.Sum(nil)), nil
}

// SetL2AuthHeader 设置 L2 认证请求头
func (auth AuthInfo) SetL2AuthHeader(req *http.Request, body []byte) error {
	if !auth.HasL2Auth() {
		return fmt.Errorf("%w: l2 auth info missing", ErrInvalidAuthInfo)
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)

	sign, err := auth.L2Sign(req.Method, req.URL.Path, ts, body)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSignError, err)
	}

	req.Header.Set("POLY_ADDRESS", auth.GetAddress())
	req.Header.Set("POLY_SIGNATURE", sign)
	req.Header.Set("POLY_TIMESTAMP", ts)
	req.Header.Set("POLY_API_KEY", auth.APIKey)
	req.Header.Set("POLY_PASSPHRASE", auth.Passphrase)

	return nil
}

// AddressFromPrivateKey 从私钥获取地址
//
// 输入输出都是 0x 开头的 16 进制格式
func AddressFromPrivateKey(privateKey string) (string, error) {
	// 解析私钥
	priKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}

	// 公钥转地址
	pubKey := priKey.Public().(*ecdsa.PublicKey)
	return strings.ToLower(crypto.PubkeyToAddress(*pubKey).Hex()), nil
}
