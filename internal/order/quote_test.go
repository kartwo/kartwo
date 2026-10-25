// 结算快照测试 / Checkout Quote Tests
// 功能：验证金额复核、配送范围、原子地址快照、失效回滚、并发和旧库升级
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 17:27:22
package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/kartwo/kartwo/internal/address"
	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"
)

type quoteFixture struct {
	db      *sql.DB
	service *Service
	cart    *cart.Service
	catalog *catalog.Service
	cartID  int64
	quote   DeliveryQuote
	input   CheckoutInfo
}

func newQuotedCart(t *testing.T, price, quantity, stock int64) quoteFixture {
	t.Helper()
	ctx := context.Background()
	db, service, crt, cat := setup(t)
	variant := seedVariant(t, cat, "quote-fixture", price, stock)
	id, _, err := crt.GetOrCreate(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = crt.AddItem(ctx, id, variant, quantity); err != nil {
		t.Fatal(err)
	}
	quote, err := service.QuoteDelivery(ctx, id, " cn ")
	if err != nil {
		t.Fatal(err)
	}
	input := info()
	input.AddressDetails = &address.Details{Line1: " 1 Fixture Road ", Line2: "Unit 2", City: "北京市", District: "海淀区", Region: "北京市", PostalCode: "100000"}
	input.ExpectedQuote = quote.Fingerprint
	return quoteFixture{db, service, crt, cat, id, quote, input}
}
func noCheckoutWrites(t *testing.T, db *sql.DB, cartID int64) {
	t.Helper()
	for _, table := range []string{"order", "order_item", "customer", "order_guest_access", "email_outbox"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM \"" + table + "\"").Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected %s rows=%d err=%v", table, count, err)
		}
	}
	var state string
	if err := db.QueryRow("SELECT status FROM cart WHERE id=?", cartID).Scan(&state); err != nil || state != "active" {
		t.Fatalf("cart state=%s err=%v", state, err)
	}
	var held int
	if err := db.QueryRow("SELECT COALESCE(SUM(reserved),0) FROM inventory").Scan(&held); err != nil || held != 0 {
		t.Fatalf("stock held=%d err=%v", held, err)
	}
}

func TestDeliveryQuoteIsReadOnlyAndPreservesFreeThreshold(t *testing.T) {
	f := newQuotedCart(t, 1000, 2, 10)
	if f.quote.Country != "CN" || f.quote.Currency != "USD" || f.quote.SubtotalCents != 2000 || f.quote.ShippingCents != 500 || f.quote.TotalCents != 2500 || len(f.quote.Fingerprint) != 64 {
		t.Fatalf("incorrect quote: %+v", f.quote)
	}
	noCheckoutWrites(t, f.db, f.cartID)
	if _, err := f.db.Exec("UPDATE shipping_zone SET free_over_cents=2000"); err != nil {
		t.Fatal(err)
	}
	next, err := f.service.QuoteDelivery(context.Background(), f.cartID, "CN")
	if err != nil || next.ShippingCents != 0 || next.TotalCents != 2000 || next.Fingerprint == f.quote.Fingerprint {
		t.Fatalf("free threshold not authoritative: %+v %v", next, err)
	}
	noCheckoutWrites(t, f.db, f.cartID)
}

func TestStructuredAddressSavedWithOrder(t *testing.T) {
	f := newQuotedCart(t, 1000, 1, 3)
	ctx := context.Background()
	id, token, err := f.service.CheckoutForGuest(ctx, f.cartID, f.input)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.service.AdminGet(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if out.ShipAddressDetails == nil || out.ShipAddressDetails.Line1 != "1 Fixture Road" || out.ShipAddressDetails.City != "北京市" || out.ShipAddressDetails.PostalCode != "100000" || out.ShipAddressDetails.District != "海淀区" || out.ShipCountry != "CN" || !strings.Contains(out.ShipAddress, "北京市") || out.TotalCents != f.quote.TotalCents {
		t.Fatalf("address/amount snapshot missing: %+v", out)
	}
	if valid, err := f.service.GuestAuthorized(ctx, id, token); err != nil || !valid {
		t.Fatalf("atomic guest grant unavailable: %v", err)
	}
	if _, err = f.service.QuoteDelivery(ctx, f.cartID, "CN"); !errors.Is(err, ErrCartConverted) {
		t.Fatalf("converted cart quote: %v", err)
	}
}

func TestReviewedCheckoutRejectsChangedSnapshotWithoutWrites(t *testing.T) {
	changes := map[string]string{
		"price":      "UPDATE variant SET price_cents=1100",
		"quantity":   "UPDATE cart_item SET quantity=2",
		"rate":       "UPDATE shipping_zone SET rate_cents=700",
		"threshold":  "UPDATE shipping_zone SET free_over_cents=1000",
		"rule-name":  "UPDATE shipping_zone SET name='Updated delivery'",
		"currency":   "INSERT INTO setting(key,value) VALUES('market.code','EU')",
		"item-title": "UPDATE product SET title='New product description'",
	}
	for name, statement := range changes {
		t.Run(name, func(t *testing.T) {
			f := newQuotedCart(t, 1000, 1, 5)
			if _, err := f.db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			if _, _, err := f.service.CheckoutForGuest(context.Background(), f.cartID, f.input); !errors.Is(err, ErrQuoteChanged) {
				t.Fatalf("changed quote accepted: %v", err)
			}
			noCheckoutWrites(t, f.db, f.cartID)
			fresh, err := f.service.QuoteDelivery(context.Background(), f.cartID, "CN")
			if err != nil {
				t.Fatal(err)
			}
			f.input.ExpectedQuote = fresh.Fingerprint
			id, err := f.service.Checkout(context.Background(), f.cartID, f.input)
			if err != nil {
				t.Fatal(err)
			}
			out, err := f.service.Get(context.Background(), id)
			if err != nil || out.TotalCents != fresh.TotalCents || out.Currency != fresh.Currency {
				t.Fatalf("fresh snapshot not saved: %v", err)
			}
		})
	}
}

func TestDeliveryCountryScopeCannotBeBypassedByGlobalFallback(t *testing.T) {
	f := newQuotedCart(t, 1000, 1, 2)
	ctx := context.Background()
	if _, err := f.service.QuoteDelivery(ctx, f.cartID, "US"); !errors.Is(err, ErrNoShippingRule) {
		t.Fatalf("unselected country accepted: %v", err)
	}
	if _, err := f.db.Exec("DELETE FROM setting WHERE key=?", ShippingCountriesSetting); err != nil {
		t.Fatal(err)
	}
	if countries, err := f.service.ShippingCountries(ctx); err != nil || len(countries) != 0 {
		t.Fatalf("global expanded country list: %+v %v", countries, err)
	}
	if _, err := f.service.QuoteDelivery(ctx, f.cartID, "CN"); !errors.Is(err, ErrNoShippingRule) {
		t.Fatalf("global bypass: %v", err)
	}
	if _, err := f.service.Checkout(ctx, f.cartID, info()); !errors.Is(err, ErrNoShippingRule) {
		t.Fatalf("legacy checkout bypass: %v", err)
	}
	if _, err := f.db.Exec("INSERT INTO shipping_zone(public_id,name,countries,rate_cents,free_over_cents) VALUES('quote-us','US',' us ',900,0)"); err != nil {
		t.Fatal(err)
	}
	quote, err := f.service.QuoteDelivery(ctx, f.cartID, "US")
	if err != nil || quote.ShippingCents != 900 {
		t.Fatalf("legacy explicit scope unavailable: %+v %v", quote, err)
	}
	if err = settings.New(f.db).SetPlain(ctx, ShippingCountriesSetting, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.QuoteDelivery(ctx, f.cartID, "US"); !errors.Is(err, ErrNoShippingRule) {
		t.Fatalf("saved empty scope ignored: %v", err)
	}
	noCheckoutWrites(t, f.db, f.cartID)
}

func TestQuoteAndCheckoutWithoutCartRemainEmpty(t *testing.T) {
	_, service, _, _ := setup(t)
	ctx := context.Background()
	for _, id := range []int64{0, 999999} {
		if _, err := service.QuoteDelivery(ctx, id, "CN"); !errors.Is(err, ErrEmptyCart) {
			t.Fatalf("absent cart quote=%v", err)
		}
		if _, err := service.Checkout(ctx, id, info()); !errors.Is(err, ErrEmptyCart) {
			t.Fatalf("absent cart checkout=%v", err)
		}
	}
}

func TestAddressProfilesCoverShippingCatalog(t *testing.T) {
	for _, country := range CountryOptions() {
		profile, ok := address.ForCountry(country.Code)
		if !ok || profile.Country != country.Code || len(profile.Fields) < 2 {
			t.Fatalf("missing address profile for catalog destination %s", country.Code)
		}
	}
}

func TestCheckoutReadsDeliveryInsideWriterSnapshot(t *testing.T) {
	f := newQuotedCart(t, 1000, 1, 2)
	// Deterministically change a rate at the write boundary. Reading rates
	// before BeginTx would accept the old quote and miss this in-transaction change.
	if _, err := f.db.Exec(`CREATE TRIGGER change_rate_on_claim AFTER UPDATE OF status ON cart WHEN NEW.status='converted' BEGIN UPDATE shipping_zone SET rate_cents=rate_cents+100; END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Checkout(context.Background(), f.cartID, f.input); !errors.Is(err, ErrQuoteChanged) {
		t.Fatalf("old pre-transaction quote accepted: %v", err)
	}
	noCheckoutWrites(t, f.db, f.cartID)
	var rate int64
	if err := f.db.QueryRow("SELECT rate_cents FROM shipping_zone").Scan(&rate); err != nil || rate != 500 {
		t.Fatalf("rate mutation did not roll back: %d %v", rate, err)
	}
}

func TestStructuredAddressFailureRollsBackEntireCheckout(t *testing.T) {
	f := newQuotedCart(t, 1000, 1, 2)
	ctx := context.Background()
	if _, err := f.db.Exec(`CREATE TRIGGER fail_address BEFORE UPDATE OF ship_address_version ON "order" BEGIN SELECT RAISE(ABORT,'injected address failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.CheckoutForGuest(ctx, f.cartID, f.input); err == nil {
		t.Fatal("address write failure accepted")
	}
	noCheckoutWrites(t, f.db, f.cartID)
	if _, err := f.db.Exec("DROP TRIGGER fail_address"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.CheckoutForGuest(ctx, f.cartID, f.input); err != nil {
		t.Fatalf("original quote unusable after rollback: %v", err)
	}
}

func TestStructuredAddressRequiresReviewedQuoteAndPrivateValidation(t *testing.T) {
	for _, name := range []string{"missing-quote", "invalid-quote", "invalid-postal", "name-control", "phone-limit", "address-control"} {
		t.Run(name, func(t *testing.T) {
			f := newQuotedCart(t, 1000, 1, 2)
			expected := ErrInvalidInfo
			switch name {
			case "missing-quote":
				f.input.ExpectedQuote = ""
				expected = ErrQuoteChanged
			case "invalid-quote":
				f.input.ExpectedQuote = "forged"
				expected = ErrQuoteChanged
			case "invalid-postal":
				f.input.AddressDetails.PostalCode = "PRIVATE-POSTAL"
			case "name-control":
				f.input.Name = "PRIVATE\r\nNAME"
			case "phone-limit":
				f.input.Phone = strings.Repeat("P", 65)
			case "address-control":
				f.input.AddressDetails.Line2 = "PRIVATE\n"
			}
			_, err := f.service.Checkout(context.Background(), f.cartID, f.input)
			if !errors.Is(err, expected) || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("validation/error privacy: %v", err)
			}
			noCheckoutWrites(t, f.db, f.cartID)
		})
	}
}

func TestCheckoutQuoteOverflowAndUnavailableItems(t *testing.T) {
	for name, statement := range map[string]string{
		"multiply":          fmt.Sprintf("UPDATE variant SET price_cents=%d; UPDATE cart_item SET quantity=2", math.MaxInt64),
		"shipping-addition": fmt.Sprintf("UPDATE variant SET price_cents=%d; UPDATE shipping_zone SET free_over_cents=0", math.MaxInt64),
		"archived":          "UPDATE product SET status='archived'",
		"product-deleted":   "UPDATE product SET deleted_at='2026-10-25T00:00:00Z'",
		"variant-deleted":   "UPDATE variant SET deleted_at='2026-10-25T00:00:00Z'",
	} {
		t.Run(name, func(t *testing.T) {
			f := newQuotedCart(t, 1000, 1, 3)
			if _, err := f.db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			expected := ErrOutOfStock
			if name == "multiply" || name == "shipping-addition" {
				expected = ErrInvalidInfo
			}
			if _, err := f.service.QuoteDelivery(context.Background(), f.cartID, "CN"); !errors.Is(err, expected) {
				t.Fatalf("unsafe quote: %v", err)
			}
			if _, err := f.service.Checkout(context.Background(), f.cartID, f.input); !errors.Is(err, expected) {
				t.Fatalf("unsafe checkout: %v", err)
			}
			noCheckoutWrites(t, f.db, f.cartID)
			if expected == ErrOutOfStock {
				var variant string
				if err := f.db.QueryRow("SELECT public_id FROM variant ORDER BY id LIMIT 1").Scan(&variant); err != nil {
					t.Fatal(err)
				}
				if err := f.cart.RemoveItem(context.Background(), f.cartID, variant); err != nil {
					t.Fatalf("unavailable item trapped the cart: %v", err)
				}
				if _, err := f.service.QuoteDelivery(context.Background(), f.cartID, "CN"); !errors.Is(err, ErrEmptyCart) {
					t.Fatalf("cart not clear after removal: %v", err)
				}
			}
		})
	}
}

func TestReviewedCheckoutConcurrentSameCart(t *testing.T) {
	f := newQuotedCart(t, 1000, 1, 3)
	var success atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.service.CheckoutForGuest(context.Background(), f.cartID, f.input)
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, ErrCartConverted) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 {
		t.Fatalf("created %d orders", success.Load())
	}
	for _, table := range []string{"order", "order_guest_access"} {
		var count int
		if err := f.db.QueryRow("SELECT count(*) FROM \"" + table + "\"").Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	_, reserved := reservedOf(t, f.db, "quote-fixture")
	if reserved != 1 {
		t.Fatalf("reserved=%d", reserved)
	}
}

func TestQuoteDatabaseErrorDoesNotDefaultCurrency(t *testing.T) {
	f := newQuotedCart(t, 1000, 1, 3)
	if _, err := f.db.Exec("DROP TABLE setting"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.QuoteDelivery(context.Background(), f.cartID, "CN"); err == nil {
		t.Fatal("database failure silently used default currency")
	}
	if _, err := f.service.Checkout(context.Background(), f.cartID, f.input); err == nil {
		t.Fatal("database failure created an order")
	}
	noCheckoutWrites(t, f.db, f.cartID)
}

func TestQuoteBindsItemAndCountryEvenWhenAmountMatches(t *testing.T) {
	t.Run("different-item-same-price", func(t *testing.T) {
		f := newQuotedCart(t, 1000, 1, 3)
		ctx := context.Background()
		var old string
		if err := f.db.QueryRow("SELECT public_id FROM variant ORDER BY id LIMIT 1").Scan(&old); err != nil {
			t.Fatal(err)
		}
		next := seedVariant(t, f.catalog, "different-quoted-item", 1000, 3)
		if err := f.cart.RemoveItem(ctx, f.cartID, old); err != nil {
			t.Fatal(err)
		}
		if err := f.cart.AddItem(ctx, f.cartID, next, 1); err != nil {
			t.Fatal(err)
		}
		quote, err := f.service.QuoteDelivery(ctx, f.cartID, "CN")
		if err != nil || quote.TotalCents != f.quote.TotalCents || quote.Fingerprint == f.quote.Fingerprint {
			t.Fatalf("item comparison missing: %v", err)
		}
		if _, err = f.service.Checkout(ctx, f.cartID, f.input); !errors.Is(err, ErrQuoteChanged) {
			t.Fatalf("changed item accepted: %v", err)
		}
		noCheckoutWrites(t, f.db, f.cartID)
	})
	t.Run("different-country-same-price", func(t *testing.T) {
		f := newQuotedCart(t, 1000, 1, 3)
		ctx := context.Background()
		if err := settings.New(f.db).SetPlain(ctx, ShippingCountriesSetting, "CN,US"); err != nil {
			t.Fatal(err)
		}
		f.input.Country = "US"
		f.input.AddressDetails.PostalCode = "94043"
		quote, err := f.service.QuoteDelivery(ctx, f.cartID, "US")
		if err != nil || quote.TotalCents != f.quote.TotalCents || quote.Fingerprint == f.quote.Fingerprint {
			t.Fatalf("country comparison missing: %v", err)
		}
		if _, err = f.service.Checkout(ctx, f.cartID, f.input); !errors.Is(err, ErrQuoteChanged) {
			t.Fatalf("changed country accepted: %v", err)
		}
		noCheckoutWrites(t, f.db, f.cartID)
	})
}

func TestQuoteZeroAmountAndSumOverflow(t *testing.T) {
	t.Run("explicit-zero", func(t *testing.T) {
		f := newQuotedCart(t, 1000, 1, 3)
		ctx := context.Background()
		if _, err := f.db.Exec("UPDATE variant SET price_cents=0; UPDATE shipping_zone SET rate_cents=0"); err != nil {
			t.Fatal(err)
		}
		quote, err := f.service.QuoteDelivery(ctx, f.cartID, "CN")
		if err != nil || quote.TotalCents != 0 || quote.SubtotalCents != 0 || quote.ShippingCents != 0 {
			t.Fatalf("zero quote rejected: %v", err)
		}
		f.input.ExpectedQuote = quote.Fingerprint
		if _, err = f.service.Checkout(ctx, f.cartID, f.input); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("sum-overflow", func(t *testing.T) {
		f := newQuotedCart(t, 1000, 1, 3)
		ctx := context.Background()
		next := seedVariant(t, f.catalog, "overflow-second", 1000, 3)
		if err := f.cart.AddItem(ctx, f.cartID, next, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec("UPDATE variant SET price_cents=?", int64(math.MaxInt64/2+1)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.QuoteDelivery(ctx, f.cartID, "CN"); !errors.Is(err, ErrInvalidInfo) {
			t.Fatalf("sum overflow accepted: %v", err)
		}
		noCheckoutWrites(t, f.db, f.cartID)
	})
}

func TestAddressMigrationKeepsLegacySnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/legacy.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	old := fstest.MapFS{}
	pending := 0
	if err = fs.WalkDir(migrations.FS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && path >= "0037" {
			pending++
			return nil
		}
		if !entry.IsDir() {
			data, readErr := fs.ReadFile(migrations.FS, path)
			if readErr != nil {
				return readErr
			}
			old[path] = &fstest.MapFile{Data: data}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = migrate.Run(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO customer(id,public_id,email,name) VALUES(1,'legacy-address-customer','legacy@example.invalid','Legacy'); INSERT INTO "order"(public_id,customer_id,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES('legacy-address-order',1,'legacy@example.invalid','Legacy','Original unsplit address, keep punctuation and 00123',100,100)`); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Run(ctx, db, migrations.FS); err != nil || applied != pending {
		t.Fatalf("address migration applied=%d err=%v", applied, err)
	}
	out, err := New(db, settings.New(db)).Get(ctx, "legacy-address-order")
	if err != nil || out.ShipAddressDetails != nil || out.ShipAddress != "Original unsplit address, keep punctuation and 00123" || out.TotalCents != 100 {
		t.Fatalf("legacy address changed: %v", err)
	}
	if applied, err := migrate.Run(ctx, db, migrations.FS); err != nil || applied != 0 {
		t.Fatalf("migration reentry=%d err=%v", applied, err)
	}
}
