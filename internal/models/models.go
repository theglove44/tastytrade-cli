// Package models contains typed structs for all TastyTrade API responses.
//
// CRITICAL: All monetary and quantity fields use shopspring/decimal.
// TastyTrade returns these values as JSON strings — never parse into float64.
package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// ── Auth ─────────────────────────────────────────────────────────────────────

// TokenResponse is the /oauth/token response payload.
// RefreshToken may be absent on some error paths — callers must check for empty
// string and retain the existing token rather than overwriting with empty.
type TokenResponse struct {
	AccessToken  string `json:"access-token"`
	TokenType    string `json:"token-type"`   // read dynamically — do not hardcode "Bearer"
	RefreshToken string `json:"refresh-token"` // may be empty — see note above
	ExpiresIn    int    `json:"expires-in"`    // seconds, currently 900
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
	UpdatedAt              time.Time        `json:"updated-at"`
}

// ── Positions ────────────────────────────────────────────────────────────────

type Position struct {
	AccountNumber        string          `json:"account-number"`
	Symbol               string          `json:"symbol"`
	InstrumentType       string          `json:"instrument-type"` // Equity, Equity Option, Future Option, …
	UnderlyingSymbol     string          `json:"underlying-symbol"`
	Quantity             decimal.Decimal `json:"quantity"`
	QuantityDirection    string          `json:"quantity-direction"` // Long / Short
	ClosePrice           decimal.Decimal `json:"close-price"`
	AverageOpenPrice     decimal.Decimal `json:"average-open-price"`
	AverageYearlyMarket  decimal.Decimal `json:"average-yearly-market-close-price"`
	MultiplierQuantity   decimal.Decimal `json:"multiplier-quantity"`
	CostEffect           string          `json:"cost-effect"` // Debit / Credit
	DayChange            decimal.Decimal `json:"realized-day-gain"`
	UnrealizedDayChange  decimal.Decimal `json:"unrealized-day-gain"`
	ExpiresAt            *time.Time       `json:"expires-at,omitempty"`
	UpdatedAt            time.Time        `json:"updated-at"`

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
	ID              string          `json:"id"`
	AccountNumber   string          `json:"account-number"`
	Status          string          `json:"status"` // Received, Routed, Live, Filled, Cancelled, …
	OrderType       string          `json:"order-type"`
	TimeInForce     string          `json:"time-in-force"`
	Price           decimal.Decimal `json:"price"`
	PriceEffect     string          `json:"price-effect"` // Debit / Credit
	Legs            []OrderLeg      `json:"legs"`
	CancelledAt     *time.Time      `json:"cancelled-at,omitempty"`
	FilledAt        *time.Time      `json:"filled-at,omitempty"`
	ReceivedAt      time.Time       `json:"received-at"`
	UpdatedAt       time.Time       `json:"updated-at"`
}

// ── Dry-Run ──────────────────────────────────────────────────────────────────

type DryRunFee struct {
	Name   string          `json:"name"`
	Amount decimal.Decimal `json:"amount"`
	Effect string          `json:"effect"` // Debit / Credit
}

type DryRunResult struct {
	Order               Order          `json:"order"`
	Errors              []DryRunError  `json:"errors,omitempty"`
	Warnings            []DryRunError  `json:"warnings,omitempty"`
	BuyingPowerEffect   BPEffect       `json:"buying-power-effect"`
	FeeCalculation      []DryRunFee    `json:"fee-calculation,omitempty"`
}

type DryRunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BPEffect struct {
	ChangeInMarginRequirement        decimal.Decimal `json:"change-in-margin-requirement"`
	ChangeInMarginRequirementEffect  string          `json:"change-in-margin-requirement-effect"`
	ChangeInBuyingPower              decimal.Decimal `json:"change-in-buying-power"`
	ChangeInBuyingPowerEffect        string          `json:"change-in-buying-power-effect"`
	CurrentBuyingPower               decimal.Decimal `json:"current-buying-power"`
	NewBuyingPower                   decimal.Decimal `json:"new-buying-power"`
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
	Price       string        `json:"price"`       // string per API convention
	PriceEffect string        `json:"price-effect"` // Debit or Credit
	Legs        []NewOrderLeg `json:"legs"`
}

// ── Quote Token ──────────────────────────────────────────────────────────────

type QuoteToken struct {
	Token          string `json:"token"`
	DxlinkURL      string `json:"dxlink-url"`
	WebSocketURL   string `json:"websocket-url"`
	Level          string `json:"level"`
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
	PageOffset    int `json:"page-offset"`
	PerPage       int `json:"per-page"`
	TotalPages    int `json:"total-pages"`
	TotalItems    int `json:"total-items"`
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
