// Package models contains typed structs for all TastyTrade API responses.
//
// CRITICAL: All monetary and quantity fields use shopspring/decimal.
// TastyTrade returns these values as JSON strings — never parse into float64.
package models

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

// ── Auth ─────────────────────────────────────────────────────────────────────

// TokenResponse is the /oauth/token response payload.
//
// The OAuth 2.0 /oauth/token endpoint returns a flat JSON object with
// underscore-separated field names — NOT the dashed names used elsewhere in
// the TastyTrade API, and NOT wrapped in a DataEnvelope {"data": {...}}.
// This matches the Python SDK (tastytrade-sdk-python) and RFC 6749 §5.1.
//
// RefreshToken may be absent on some error paths — callers must check for empty
// string and retain the existing token rather than overwriting with empty.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`    // read dynamically — do not hardcode "Bearer"
	RefreshToken string `json:"refresh_token"` // may be empty — see note above
	ExpiresIn    int    `json:"expires_in"`    // seconds, currently 900
}

// ── Accounts ─────────────────────────────────────────────────────────────────

type Customer struct {
	ID        string `json:"id"`
	FirstName string `json:"first-name"`
	LastName  string `json:"last-name"`
	Email     string `json:"email"`
}

type Account struct {
	AccountNumber string `json:"account-number"`
	AccountType   string `json:"account-type"`
	Nickname      string `json:"nickname"`
	IsClosed      bool   `json:"is-closed"`
	IsFirmError   bool   `json:"is-firm-error"`
}

// AccountListItem is the list-item wrapper returned by GET /customers/me/accounts.
// The endpoint returns data.items[].account rather than data.items[] being the
// account object directly.
type AccountListItem struct {
	Account Account `json:"account"`
}

// ── Balances ─────────────────────────────────────────────────────────────────

type Balance struct {
	AccountNumber          string          `json:"account-number"`
	CashBalance            decimal.Decimal `json:"cash-balance"`
	LongEquityValue        decimal.Decimal `json:"long-equity-value"`
	ShortEquityValue       decimal.Decimal `json:"short-equity-value"`
	LongDerivativeValue    decimal.Decimal `json:"long-derivative-value"`
	ShortDerivativeValue   decimal.Decimal `json:"short-derivative-value"`
	LongFuturesValue       decimal.Decimal `json:"long-futures-value"`
	ShortFuturesValue      decimal.Decimal `json:"short-futures-value"`
	NetLiquidatingValue    decimal.Decimal `json:"net-liquidating-value"`
	BuyingPower            decimal.Decimal `json:"equity-buying-power"`
	MaintenanceRequirement decimal.Decimal `json:"maintenance-requirement"`
	MaintenanceCallValue   decimal.Decimal `json:"maintenance-call-value"`
	RegTMarginRequirement  decimal.Decimal `json:"reg-t-margin-requirement"`
	DayTradeBuyingPower    decimal.Decimal `json:"day-trade-buying-power"`
	UpdatedAt              time.Time       `json:"updated-at"`
}

// ── Positions ────────────────────────────────────────────────────────────────

type Position struct {
	AccountNumber       string          `json:"account-number"`
	Symbol              string          `json:"symbol"`
	InstrumentType      string          `json:"instrument-type"` // Equity, Equity Option, Future Option, …
	UnderlyingSymbol    string          `json:"underlying-symbol"`
	Quantity            decimal.Decimal `json:"quantity"`
	QuantityDirection   string          `json:"quantity-direction"` // Long / Short
	ClosePrice          decimal.Decimal `json:"close-price"`
	AverageOpenPrice    decimal.Decimal `json:"average-open-price"`
	AverageYearlyMarket decimal.Decimal `json:"average-yearly-market-close-price"`
	MultiplierQuantity  decimal.Decimal `json:"multiplier-quantity"`
	CostEffect          string          `json:"cost-effect"` // Debit / Credit
	DayChange           decimal.Decimal `json:"realized-day-gain"`
	UnrealizedDayChange decimal.Decimal `json:"unrealized-day-gain"`
	ExpiresAt           *time.Time      `json:"expires-at,omitempty"`
	UpdatedAt           time.Time       `json:"updated-at"`

	// Greeks — populated when streamer data is merged.
	Delta *float64 `json:"-"`
	Theta *float64 `json:"-"`
}

// ── Orders ───────────────────────────────────────────────────────────────────

type OrderLeg struct {
	InstrumentType string          `json:"instrument-type"`
	Symbol         string          `json:"symbol"`
	Quantity       decimal.Decimal `json:"quantity"`
	Action         string          `json:"action"` // Buy to Open, Sell to Close, …
	FillQuantity   decimal.Decimal `json:"fill-quantity,omitempty"`
	FillPrice      decimal.Decimal `json:"average-fill-price,omitempty"`
}

type Order struct {
	ID            string          `json:"id"`
	AccountNumber string          `json:"account-number"`
	Status        string          `json:"status"` // Received, Routed, Live, Filled, Cancelled, …
	OrderType     string          `json:"order-type"`
	TimeInForce   string          `json:"time-in-force"`
	Price         decimal.Decimal `json:"price"`
	PriceEffect   string          `json:"price-effect"` // Debit / Credit
	Legs          []OrderLeg      `json:"legs"`
	CancelledAt   *time.Time      `json:"cancelled-at,omitempty"`
	FilledAt      *time.Time      `json:"filled-at,omitempty"`
	ReceivedAt    time.Time       `json:"received-at"`
	UpdatedAt     time.Time       `json:"updated-at"`
}

// ── Dry-Run ──────────────────────────────────────────────────────────────────

type DryRunFee struct {
	Name   string          `json:"name"`
	Amount decimal.Decimal `json:"amount"`
	Effect string          `json:"effect"` // Debit / Credit
}

type DryRunResult struct {
	Order             Order         `json:"order"`
	Errors            []DryRunError `json:"errors,omitempty"`
	Warnings          []DryRunError `json:"warnings,omitempty"`
	BuyingPowerEffect BPEffect      `json:"buying-power-effect"`
	FeeCalculation    []DryRunFee   `json:"fee-calculation,omitempty"`
}

type DryRunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BPEffect struct {
	ChangeInMarginRequirement       decimal.Decimal `json:"change-in-margin-requirement"`
	ChangeInMarginRequirementEffect string          `json:"change-in-margin-requirement-effect"`
	ChangeInBuyingPower             decimal.Decimal `json:"change-in-buying-power"`
	ChangeInBuyingPowerEffect       string          `json:"change-in-buying-power-effect"`
	CurrentBuyingPower              decimal.Decimal `json:"current-buying-power"`
	NewBuyingPower                  decimal.Decimal `json:"new-buying-power"`
}

// ── New Order Request ─────────────────────────────────────────────────────────

type NewOrderLeg struct {
	InstrumentType string `json:"instrument-type"`
	Symbol         string `json:"symbol"`
	Quantity       int    `json:"quantity"`
	Action         string `json:"action"`
}

type NewOrder struct {
	OrderType   string        `json:"order-type"`
	TimeInForce string        `json:"time-in-force"`
	Price       string        `json:"price"`        // string per API convention
	PriceEffect string        `json:"price-effect"` // Debit or Credit
	Legs        []NewOrderLeg `json:"legs"`
}

// ── Quote Token ──────────────────────────────────────────────────────────────

// QuoteToken is the response payload from GET /api-quote-tokens.
// This endpoint is UNVERSIONED — always use RequestOptions{SkipVersion: true}.
// The token must be re-fetched before every DXLink connect and reconnect.
type QuoteToken struct {
	Token        string `json:"token"`
	DxlinkURL    string `json:"dxlink-url"`    // preferred WS endpoint from server
	WebSocketURL string `json:"websocket-url"` // fallback
	Level        string `json:"level"`
	ExpiresIn    int    `json:"expires-in"` // seconds; 0 if not provided
}

// ── Market Streamer Wire Types ────────────────────────────────────────────────
//
// DXLink wire protocol (spec §1.6).
// All messages are JSON objects with a "type" discriminator field.
// Channel 0 is the control channel; data channels are assigned by the server.
//
// Handshake sequence (per connect/reconnect):
//  1. Dial WebSocket (URL from QuoteToken.DxlinkURL or config fallback)
//  2. Send SETUP:            {"type":"SETUP","channel":0,"version":"0.1","keepaliveTimeout":60,"acceptKeepaliveTimeout":60}
//  3. Receive SETUP response
//  4. Send AUTH:             {"type":"AUTH","channel":0,"token":"<QuoteToken.Token>"}
//  5. Receive AUTH_STATE:    {"type":"AUTH_STATE","channel":0,"state":"AUTHORIZED"}
//  6. Send CHANNEL_REQUEST:  {"type":"CHANNEL_REQUEST","channel":1,"service":"FEED","parameters":{"contract":"AUTO"}}
//  7. Receive CHANNEL_OPENED
//  8. Send FEED_SETUP:       {"type":"FEED_SETUP","channel":1,"acceptAggregationPeriod":0.1,"acceptDataFormat":"COMPACT","acceptEventFields":{"Quote":["eventType","eventSymbol","bidPrice","askPrice","lastPrice","time"]}}
//  9. Send FEED_SUBSCRIPTION: {"type":"FEED_SUBSCRIPTION","channel":1,"add":[{"type":"Quote","symbol":"<sym>"},...]}
// 10. Loop: receive FEED_DATA, send KEEPALIVE on channel 0 every 30s

// DXLinkMsg is the minimal envelope used to peek at the "type" field.
type DXLinkMsg struct {
	Type    string `json:"type"`
	Channel int    `json:"channel"`
}

// DXLinkFeedData carries one or more compact quote rows.
// Data is an array of arrays — each inner array maps to the field order
// declared in FEED_SETUP's acceptEventFields.
// Compact format: ["Quote","SPY",450.10,450.11,450.05,1234567890000]
type DXLinkFeedData struct {
	Type    string            `json:"type"` // "FEED_DATA"
	Channel int               `json:"channel"`
	Data    []json.RawMessage `json:"data"` // array of compact event arrays
}

// DXLinkAuthState carries the AUTH_STATE response.
type DXLinkAuthState struct {
	Type    string `json:"type"` // "AUTH_STATE"
	Channel int    `json:"channel"`
	State   string `json:"state"` // "AUTHORIZED" | "UNAUTHORIZED"
}

// DXLinkChannelOpened carries the CHANNEL_OPENED response.
type DXLinkChannelOpened struct {
	Type    string `json:"type"`
	Channel int    `json:"channel"`
	Service string `json:"service"`
}

// ── API envelope ─────────────────────────────────────────────────────────────

// DataEnvelope wraps the standard {"data": {...}} response shape.
type DataEnvelope[T any] struct {
	Data    T      `json:"data"`
	Context string `json:"context,omitempty"`
}

// ItemsEnvelope wraps the standard {"data": {"items": [...]}} response shape.
type ItemsEnvelope[T any] struct {
	Data struct {
		Items      []T    `json:"items"`
		Pagination *Pager `json:"pagination,omitempty"`
	} `json:"data"`
	Context string `json:"context,omitempty"`
}

type Pager struct {
	PageOffset       int `json:"page-offset"`
	PerPage          int `json:"per-page"`
	TotalPages       int `json:"total-pages"`
	TotalItems       int `json:"total-items"`
	CurrentItemCount int `json:"current-item-count"`
}

// ErrorEnvelope wraps the standard error response.
type ErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Domain  string `json:"domain"`
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	} `json:"error"`
}

// ── Account Streamer Events ───────────────────────────────────────────────────
//
// The account streamer delivers JSON messages over WebSocket.
// Each message has a "type" field that routes to the appropriate concrete
// struct.  The raw "data" payload is decoded separately to avoid a large
// discriminated-union decode.
//
// Wire protocol confirmed from spec §1.5:
//   auth-token = raw access_token — NO "Bearer" prefix
//   {"action":"connect",      "value":["ACCT#"],"request-id":1,"auth-token":"..."}
//   {"action":"account-subscribe","value":["ACCT#"],"request-id":2,"auth-token":"..."}
//   {"action":"heartbeat",    "request-id":null, "auth-token":"..."}

// AccountMessage is the outer envelope for every account streamer frame.
// Type routes the Data payload to the correct concrete struct.
type AccountMessage struct {
	Type      string          `json:"type"`   // "order", "account-balance", "position", "heartbeat-ack"
	Action    string          `json:"action"` // "Snapshot", "Change"
	RequestID *int            `json:"request-id,omitempty"`
	Data      json.RawMessage `json:"data"`
}

// OrderEvent is delivered when an order status changes (including fills).
// Status=="Filled" with a non-nil FilledAt is a confirmed execution.
type OrderEvent struct {
	AccountNumber string     `json:"account-number"`
	OrderID       string     `json:"id"`
	Status        string     `json:"status"` // Received, Live, Filled, Cancelled, Rejected
	FilledAt      *time.Time `json:"filled-at,omitempty"`
	Legs          []OrderLeg `json:"legs"` // reuses existing OrderLeg
}

// BalanceEvent carries net-liquidating value and buying-power after a change.
// This is the authoritative source for NLQ guard checks and the NLQDollars metric.
type BalanceEvent struct {
	AccountNumber       string          `json:"account-number"`
	NetLiquidatingValue decimal.Decimal `json:"net-liquidating-value"`
	BuyingPower         decimal.Decimal `json:"equity-buying-power"`
	UpdatedAt           time.Time       `json:"updated-at"`
}

// PositionEvent is delivered when a position opens, changes, or closes.
// Action: "Open", "Change", "Close"
type PositionEvent struct {
	AccountNumber     string          `json:"account-number"`
	Symbol            string          `json:"symbol"`
	InstrumentType    string          `json:"instrument-type"`
	Quantity          decimal.Decimal `json:"quantity"`
	QuantityDirection string          `json:"quantity-direction"` // Long / Short
	Action            string          `json:"action"`             // Open / Change / Close
	UpdatedAt         time.Time       `json:"updated-at"`
}

// ── Market Streamer Events ────────────────────────────────────────────────────

// QuoteEvent carries bid/ask/last for a single symbol from DXLink.
// MarkPrice is derived client-side: (bid+ask)/2 when both are non-zero,
// otherwise last price. If all three are zero the quote is considered stale
// and MarkStale will be true.
type QuoteEvent struct {
	Symbol    string          `json:"eventSymbol"`
	BidPrice  decimal.Decimal `json:"bidPrice"`
	AskPrice  decimal.Decimal `json:"askPrice"`
	LastPrice decimal.Decimal `json:"lastPrice"`
	MarkPrice decimal.Decimal `json:"markPrice"` // derived — see above
	MarkStale bool            `json:"markStale"` // true when mark cannot be determined
	EventTime time.Time       `json:"time"`
}
