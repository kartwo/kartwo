// 结算运费快照 / Checkout Delivery Quote
// 功能：同一读取快照计算金额，提交时事务内复核购物车/国家/运费，拒绝静默涨价或溢出
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 17:22:42
package order

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var ErrQuoteChanged = errors.New("order: 购物车或配送金额已变化，请重新确认")

// Fingerprint is a comparison precondition, not an authorization token or an
// accepted client price. Every amount is recalculated from SQL before ordering.
type DeliveryQuote struct {
	Country          string
	Currency         string
	SubtotalCents    int64
	ShippingCents    int64
	TotalCents       int64
	ShippingRuleName string
	Lines            []OrderLine
	Fingerprint      string
}

func quoteSnapshot(ctx context.Context, db sqlcgen.DBTX, cartID int64, country string) (DeliveryQuote, []sqlcgen.ListCartItemsRow, error) {
	quote := DeliveryQuote{Country: strings.ToUpper(strings.TrimSpace(country))}
	q := sqlcgen.New(db)
	items, err := q.ListCartItems(ctx, cartID)
	if err != nil {
		return quote, nil, err
	}
	if len(items) == 0 {
		return quote, nil, ErrEmptyCart
	}
	for _, item := range items {
		if item.ProductStatus != "active" || item.ProductDeletedAt.Valid || item.VariantDeletedAt.Valid {
			return quote, nil, ErrOutOfStock
		}
		if item.Quantity <= 0 || item.PriceCents < 0 || item.PriceCents > math.MaxInt64/item.Quantity {
			return quote, nil, ErrInvalidInfo
		}
		line := item.PriceCents * item.Quantity
		if line > math.MaxInt64-quote.SubtotalCents {
			return quote, nil, ErrInvalidInfo
		}
		quote.SubtotalCents += line
		label, labelErr := variantLabel(ctx, q, item.VariantID)
		if labelErr != nil {
			return quote, nil, labelErr
		}
		sku := ""
		if item.Sku.Valid {
			sku = item.Sku.String
		}
		quote.Lines = append(quote.Lines, OrderLine{Title: item.ProductTitle, Spec: label, SKU: sku, UnitCents: item.PriceCents, Quantity: item.Quantity, LineCents: line})
	}
	quote.Currency, err = settings.CurrencySnapshot(ctx, q)
	if err != nil {
		return quote, nil, err
	}
	state, err := loadShipping(ctx, db)
	if err != nil {
		return quote, nil, err
	}
	quote.ShippingCents, quote.ShippingRuleName, err = state.forCountry(quote.Country, quote.SubtotalCents)
	if err != nil {
		return quote, nil, err
	}
	if quote.ShippingCents > math.MaxInt64-quote.SubtotalCents {
		return quote, nil, ErrInvalidInfo
	}
	quote.TotalCents = quote.SubtotalCents + quote.ShippingCents
	payload, err := json.Marshal(struct {
		CartID int64
		Quote  DeliveryQuote
		Items  []sqlcgen.ListCartItemsRow
	}{cartID, quote, items})
	if err != nil {
		return quote, nil, err
	}
	digest := sha256.Sum256(payload)
	quote.Fingerprint = hex.EncodeToString(digest[:])
	return quote, items, nil
}

// QuoteDelivery is read-only: no stock reservation, order, guest grant or mail.
func (s *Service) QuoteDelivery(ctx context.Context, cartID int64, country string) (DeliveryQuote, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return DeliveryQuote{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var state string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM cart WHERE id=?", cartID).Scan(&state); errors.Is(err, sql.ErrNoRows) {
		return DeliveryQuote{}, ErrEmptyCart
	} else if err != nil {
		return DeliveryQuote{}, err
	}
	if state != "active" {
		return DeliveryQuote{}, ErrCartConverted
	}
	quote, _, err := quoteSnapshot(ctx, tx, cartID, country)
	if err != nil {
		return DeliveryQuote{}, err
	}
	if err = tx.Commit(); err != nil {
		return DeliveryQuote{}, err
	}
	return quote, nil
}
