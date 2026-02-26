// 本文件主要内容：封装 freqtrade forceenter/forceexit 与 trades 查询。

package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"brale-core/internal/pkg/httpclient"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parseutil"

	"github.com/hashicorp/go-retryablehttp"
	"go.uber.org/zap"
)

type FreqtradeClient struct {
	Endpoint    string
	Client      *http.Client
	RetryClient *retryablehttp.Client
	APIKey      string
	APISecret   string
	AuthType    string

	initOnce sync.Once
}

type Float64OrNull float64

func (v *Float64OrNull) UnmarshalJSON(data []byte) error {
	n, err := parseutil.ParseNullableFloatJSON(data)
	if err != nil {
		return err
	}
	*v = Float64OrNull(n)
	return nil
}

type Int64OrNull int64

func (v *Int64OrNull) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*v = 0
		return nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = strings.Trim(s, "\"")
	}
	if s == "" {
		*v = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*v = Int64OrNull(n)
	return nil
}

type StringOrNull string

func (v *StringOrNull) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*v = ""
		return nil
	}
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*v = StringOrNull(decoded)
	return nil
}

func (c *FreqtradeClient) Balance(ctx context.Context) (map[string]any, error) {

	var out map[string]any
	if err := c.doGet(ctx, "/balance", &out); err != nil {
		return nil, err
	}
	return out, nil
}

type ForceEnterPayload struct {
	Pair        string   `json:"pair"`
	Side        string   `json:"side"`
	Price       *float64 `json:"price,omitempty"`
	OrderType   string   `json:"ordertype,omitempty"`
	StakeAmount float64  `json:"stakeamount,omitempty"`
	EntryTag    string   `json:"entry_tag,omitempty"`
	Leverage    float64  `json:"leverage,omitempty"`
}

type ForceEnterResponse struct {
	TradeID int `json:"trade_id"`
}

type ForceExitPayload struct {
	TradeID   string  `json:"tradeid"`
	OrderType string  `json:"ordertype,omitempty"`
	Amount    float64 `json:"amount,omitempty"`
}

type Trade struct {
	ID                   int           `json:"trade_id"`
	Pair                 string        `json:"pair"`
	Side                 string        `json:"side"`
	IsShort              bool          `json:"is_short"`
	IsOpen               bool          `json:"is_open"`
	Amount               Float64OrNull `json:"amount"`
	AmountRequested      Float64OrNull `json:"amount_requested,omitempty"`
	StakeAmount          Float64OrNull `json:"stake_amount"`
	MaxStakeAmount       Float64OrNull `json:"max_stake_amount,omitempty"`
	OpenRate             Float64OrNull `json:"open_rate"`
	OpenRateRequested    Float64OrNull `json:"open_rate_requested,omitempty"`
	OpenTradeValue       Float64OrNull `json:"open_trade_value,omitempty"`
	CloseRate            Float64OrNull `json:"close_rate,omitempty"`
	CloseRateRequested   Float64OrNull `json:"close_rate_requested,omitempty"`
	RealizedProfit       Float64OrNull `json:"realized_profit,omitempty"`
	RealizedProfitRatio  Float64OrNull `json:"realized_profit_ratio,omitempty"`
	CloseProfitAbs       Float64OrNull `json:"close_profit_abs,omitempty"`
	CloseProfitPct       Float64OrNull `json:"close_profit_pct,omitempty"`
	ProfitAbs            Float64OrNull `json:"profit_abs,omitempty"`
	TotalProfitAbs       Float64OrNull `json:"total_profit_abs,omitempty"`
	ProfitPct            Float64OrNull `json:"profit_pct,omitempty"`
	ExitReason           StringOrNull  `json:"exit_reason,omitempty"`
	ExitOrderStatus      StringOrNull  `json:"exit_order_status,omitempty"`
	OpenFillTimestamp    Int64OrNull   `json:"open_fill_timestamp,omitempty"`
	CloseTimestamp       Int64OrNull   `json:"close_timestamp,omitempty"`
	Leverage             Float64OrNull `json:"leverage"`
	CurrentRate          Float64OrNull `json:"current_rate,omitempty"`
	EnterTag             StringOrNull  `json:"enter_tag,omitempty"`
	TradeDuration        int           `json:"trade_duration,omitempty"`
	TradeDurationSeconds Int64OrNull   `json:"trade_duration_s,omitempty"`
	OpenOrderID          StringOrNull  `json:"open_order_id,omitempty"`
	CloseOrderID         StringOrNull  `json:"close_order_id,omitempty"`
	HasOpenOrders        bool          `json:"has_open_orders,omitempty"`
	Orders               []TradeOrder  `json:"orders,omitempty"`
}

type TradeOrder struct {
	OrderID        string        `json:"order_id,omitempty"`
	Status         StringOrNull  `json:"status,omitempty"`
	Amount         Float64OrNull `json:"amount,omitempty"`
	Price          Float64OrNull `json:"price,omitempty"`
	SafePrice      Float64OrNull `json:"safe_price,omitempty"`
	Average        Float64OrNull `json:"average,omitempty"`
	Filled         Float64OrNull `json:"filled,omitempty"`
	FTOrderSide    StringOrNull  `json:"ft_order_side,omitempty"`
	OrderType      StringOrNull  `json:"order_type,omitempty"`
	IsOpen         bool          `json:"is_open,omitempty"`
	OrderTimestamp Int64OrNull   `json:"order_timestamp,omitempty"`
	OrderFilledAt  Int64OrNull   `json:"order_filled_timestamp,omitempty"`
	FTOrderTag     StringOrNull  `json:"ft_order_tag,omitempty"`
	FTIsEntry      bool          `json:"ft_is_entry,omitempty"`
	Remaining      Float64OrNull `json:"remaining,omitempty"`
	OrderCost      Float64OrNull `json:"cost,omitempty"`
	FTFeeBase      Float64OrNull `json:"ft_fee_base,omitempty"`
}

type TradeResponse struct {
	Trades      []Trade `json:"trades"`
	TradesCount int     `json:"trades_count"`
	Offset      int     `json:"offset"`
	TotalTrades int     `json:"total_trades"`
}

func (c *FreqtradeClient) ForceEnter(ctx context.Context, payload ForceEnterPayload) (*ForceEnterResponse, error) {
	var resp ForceEnterResponse
	if err := c.doRequest(ctx, http.MethodPost, "/forceenter", payload, &resp); err != nil {
		return nil, err
	}
	if resp.TradeID == 0 {
		return nil, fmt.Errorf("freqtrade empty trade_id")
	}
	return &resp, nil
}

func (c *FreqtradeClient) ForceExit(ctx context.Context, payload ForceExitPayload) error {
	return c.doRequest(ctx, http.MethodPost, "/forceexit", payload, nil)
}

func (c *FreqtradeClient) ListOpenTrades(ctx context.Context) ([]Trade, error) {
	var trades []Trade
	if err := c.doGet(ctx, "/status", &trades); err != nil {
		return nil, err
	}
	open := make([]Trade, 0, len(trades))
	for _, tr := range trades {
		if tr.IsOpen {
			open = append(open, tr)
		}
	}
	return open, nil
}

func (c *FreqtradeClient) ListTrades(ctx context.Context, limit, offset int) (TradeResponse, error) {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	var resp TradeResponse
	path := fmt.Sprintf("/trades?limit=%d&offset=%d&order_by_id=true", limit, offset)
	if err := c.doGet(ctx, path, &resp); err != nil {
		return TradeResponse{}, err
	}
	return resp, nil
}

func (c *FreqtradeClient) FindTradeByID(ctx context.Context, tradeID int) (Trade, bool, error) {
	if tradeID <= 0 {
		return Trade{}, false, fmt.Errorf("trade_id is required")
	}
	open, err := c.ListOpenTrades(ctx)
	if err != nil {
		return Trade{}, false, err
	}
	for _, tr := range open {
		if tr.ID == tradeID {
			return tr, true, nil
		}
	}
	return c.FindTradeInHistory(ctx, tradeID)
}

func (c *FreqtradeClient) FindTradeInHistory(ctx context.Context, tradeID int) (Trade, bool, error) {
	if tradeID <= 0 {
		return Trade{}, false, fmt.Errorf("trade_id is required")
	}
	offset := 0
	for {
		resp, err := c.ListTrades(ctx, 200, offset)
		if err != nil {
			return Trade{}, false, err
		}
		for _, tr := range resp.Trades {
			if tr.ID == tradeID {
				return tr, true, nil
			}
		}
		offset += len(resp.Trades)
		if len(resp.Trades) == 0 || offset >= resp.TotalTrades {
			break
		}
	}
	return Trade{}, false, nil
}

func (c *FreqtradeClient) CancelOpenOrder(ctx context.Context, tradeID int) error {
	if tradeID <= 0 {
		return fmt.Errorf("trade_id is required")
	}
	path := fmt.Sprintf("/trades/%d/open-order", tradeID)
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil)
}

func (c *FreqtradeClient) Ping(ctx context.Context) error {
	var out []Trade
	return c.doGet(ctx, "/status", &out)
}

func (c *FreqtradeClient) doRequest(ctx context.Context, method, path string, payload any, out any) error {
	if c == nil {
		return fmt.Errorf("freqtrade client is nil")
	}
	endpoint := normalizeFreqtradeEndpointBase(c.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("freqtrade endpoint is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	url := endpoint + path
	req, err := httpclient.NewJSONRequest(ctx, method, url, payload)
	if err != nil {
		return err
	}
	applyExecAuth(req, c.AuthType, c.APIKey, c.APISecret)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		logging.FromContext(ctx).Named("execution").Error("freqtrade request failed", zap.Error(err), zap.String("path", path))
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return handleFreqtradeResponseError(ctx, path, resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *FreqtradeClient) httpClient() *http.Client {
	c.initClients()
	if c != nil && c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (c *FreqtradeClient) retryClient() *retryablehttp.Client {
	c.initClients()
	if c != nil && c.RetryClient != nil {
		return c.RetryClient
	}
	if c == nil {
		return newFreqtradeRetryClient(nil)
	}
	rc := newFreqtradeRetryClient(c.httpClient())
	c.RetryClient = rc
	return rc
}

func (c *FreqtradeClient) initClients() {
	if c == nil {
		return
	}
	c.initOnce.Do(func() {
		if c.Client == nil {
			c.Client = &http.Client{Timeout: 5 * time.Second}
		}
		if c.RetryClient == nil {
			c.RetryClient = newFreqtradeRetryClient(c.Client)
		}
	})
	if c.RetryClient != nil && c.RetryClient.HTTPClient == nil {
		c.RetryClient.HTTPClient = c.Client
	}
}

func (c *FreqtradeClient) doGet(ctx context.Context, path string, out any) error {
	if c == nil {
		return fmt.Errorf("freqtrade client is nil")
	}
	endpoint := normalizeFreqtradeEndpointBase(c.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("freqtrade endpoint is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	url := endpoint + path
	req, err := retryablehttp.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/json")
	applyExecAuth(req.Request, c.AuthType, c.APIKey, c.APISecret)
	resp, err := c.retryClient().Do(req)
	if err != nil {
		logging.FromContext(ctx).Named("execution").Error("freqtrade get failed", zap.Error(err), zap.String("path", path))
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return handleFreqtradeResponseError(ctx, path, resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func handleFreqtradeResponseError(ctx context.Context, path string, resp *http.Response) error {
	bodyBytes, readErr := httpclient.ReadLimitedBody(resp.Body, 2048)
	if readErr != nil {
		return fmt.Errorf("freqtrade status %s (read body failed: %w)", resp.Status, readErr)
	}
	bodyText := strings.TrimSpace(string(bodyBytes))
	logging.FromContext(ctx).Named("execution").Warn("freqtrade response error",
		zap.String("status", resp.Status),
		zap.String("path", path),
		zap.String("body", bodyText),
	)
	if bodyText == "" {
		return fmt.Errorf("freqtrade status %s", resp.Status)
	}
	return fmt.Errorf("freqtrade status %s: %s", resp.Status, bodyText)
}
