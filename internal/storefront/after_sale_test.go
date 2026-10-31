// 售后店面边界测试 / Storefront After Sale Boundary Tests
// 功能：真实模板/表单、订单凭证、币种/金额、退回过程、来源与限额，以及未知写入恢复
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 21:42:28
package storefront

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/httpx"
	"github.com/kartwo/kartwo/internal/order"
	"golang.org/x/net/html"
)

func afterSalePageFixture(t *testing.T, shipped bool) (*HTTP, http.Handler, *http.Cookie) {
	t.Helper()
	h, mux := newHTTP(t)
	seedPrivateOrder(t, h)
	if _, err := h.svc.db.Exec(`UPDATE "order" SET status='paid' WHERE public_id='private-order'`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.db.Exec(`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency) SELECT id,'stripe','private-after-sale-payment',total_cents,currency FROM "order" WHERE public_id='private-order'`); err != nil {
		t.Fatal(err)
	}
	if shipped {
		if err := h.order.Fulfill(context.Background(), "private-order", "Outbound carrier", "OUTBOUND-PRIVATE", ""); err != nil {
			t.Fatal(err)
		}
	}
	return h, mux, authorizeGuestTest(t, h, "private-order")
}

// Read actual native HTML forms. The first matching form is the pinned recovery
// form on an uncertain write; disabled new-action forms appear afterwards.
func saleForm(t *testing.T, page *httptest.ResponseRecorder, path, action string) url.Values {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(page.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	attr := func(n *html.Node, key string) string {
		for _, a := range n.Attr {
			if a.Key == key {
				return a.Val
			}
		}
		return ""
	}
	var found url.Values
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "form" && attr(n, "action") == path {
			values := url.Values{}
			var fields func(*html.Node)
			fields = func(f *html.Node) {
				if f.Type == html.ElementNode && f.Data == "input" && attr(f, "name") != "" {
					values.Set(attr(f, "name"), attr(f, "value"))
				}
				if f.Type == html.ElementNode && f.Data == "textarea" && attr(f, "name") != "" {
					var text strings.Builder
					for c := f.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode {
							text.WriteString(c.Data)
						}
					}
					values.Set(attr(f, "name"), text.String())
				}
				for c := f.FirstChild; c != nil; c = c.NextSibling {
					fields(c)
				}
			}
			fields(n)
			if values.Get("action") == action {
				found = values
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if found == nil {
		t.Fatalf("missing form %s %s", path, action)
	}
	return found
}

func salePageCount(t *testing.T, h *HTTP, cases, events int) {
	t.Helper()
	var c, e int
	if err := h.svc.db.QueryRow(`SELECT (SELECT COUNT(*) FROM after_sale),(SELECT COUNT(*) FROM after_sale_event)`).Scan(&c, &e); err != nil || c != cases || e != events {
		t.Fatalf("cases=%d events=%d err=%v", c, e, err)
	}
}
func saleCase(t *testing.T, h *HTTP) order.AfterSaleCase {
	t.Helper()
	snapshot, err := h.order.AdminAfterSales(context.Background(), "private-order")
	if err != nil || len(snapshot.Cases) != 1 {
		t.Fatal("missing case", err)
	}
	return snapshot.Cases[0]
}

func TestAfterSaleDecimalAmountIsExactAndBounded(t *testing.T) {
	for _, tc := range []struct {
		value string
		cents int64
		ok    bool
	}{
		{"0.01", 1, true}, {"001.20", 120, true}, {"12.3", 1230, true}, {" 12.34 ", 1234, true}, {"92233720368547758.07", 9223372036854775807, true},
		{"0", 0, false}, {"-1", 0, false}, {"1.001", 0, false}, {"1e3", 0, false}, {"1,000", 0, false}, {"+1", 0, false}, {".5", 0, false}, {"1.", 0, false}, {"NaN", 0, false}, {"92233720368547758.08", 0, false}, {"99999999999999999", 0, false},
	} {
		got, ok := afterSaleCents(tc.value)
		if got != tc.cents || ok != tc.ok {
			t.Fatalf("%q cents=%d ok=%t", tc.value, got, ok)
		}
	}
}

func TestAfterSalePageGuestIsolationAndNativeSourceGuard(t *testing.T) {
	h, mux, cookie := afterSalePageFixture(t, false)
	for _, id := range []string{"private-order", "missing-order"} {
		for _, method := range []string{"GET", "POST"} {
			page := guestRequest(t, mux, method, afterSalePath(id), "amount=1&reason=guess")
			if page.Code != 200 {
				t.Fatal(page.Code)
			}
			assertNoPrivateOrder(t, page)
			if strings.Contains(page.Body.String(), "Submit request for review") {
				t.Fatal("unverified form disclosed")
			}
		}
	}
	for _, path := range []string{afterSalePath("private-order"), afterSalePath("private-order") + "/" + uuid.Must(uuid.NewV7()).String() + "/actions"} {
		for _, headers := range []map[string]string{{}, {"Origin": "null"}, {"Origin": "https://foreign.example"}, {"Origin": "https://foreign.example", "Sec-Fetch-Site": "same-origin"}, {"Sec-Fetch-Site": "same-site"}} {
			r := httptest.NewRequest("POST", path, strings.NewReader("amount=1&reason=cross-site"))
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for k, v := range headers {
				r.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			requirePrivate(t, w)
			if w.Code != 403 {
				t.Fatal("source guard bypass", w.Code)
			}
		}
	}
	page := guestRequest(t, mux, "GET", afterSalePath("second-private-order"), "", cookie)
	assertNoPrivateOrder(t, page)
	salePageCount(t, h, 0, 0)
}

func TestAfterSaleNativeReturnFlowValidationReplayAndInspection(t *testing.T) {
	h, mux, cookie := afterSalePageFixture(t, true)
	ctx := context.Background()
	path := afterSalePath("private-order")
	page := guestRequest(t, mux, "GET", path, "", cookie)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Return and refund") || strings.Contains(page.Body.String(), "within 7 days") {
		t.Fatal("native form/default policy", page.Code)
	}
	if !strings.Contains(page.Body.String(), `content="noindex,nofollow,noarchive"`) {
		t.Fatal("private meta robots conflicts with response headers")
	}
	form := saleForm(t, page, path, "")
	if !afterSaleNonce(form.Get("request_key")) {
		t.Fatal("missing UUIDv7 form nonce")
	}
	form.Set("kind", "return_refund")
	form.Set("amount", "12.34")
	form.Set("reason", "Size problem <script>window.BAD=1</script>\nReturn one item.")
	post := func(values url.Values) *httptest.ResponseRecorder {
		return guestRequest(t, mux, "POST", path, values.Encode(), cookie)
	}
	created := post(form)
	if created.Code != 303 || created.Header().Get("Location") != path {
		t.Fatal("native request submit", created.Code)
	}
	if again := post(form); again.Code != 303 {
		t.Fatal("lost response replay", again.Code)
	}
	salePageCount(t, h, 1, 1)
	c := saleCase(t, h)
	if c.AmountCents != 1234 || c.Currency != "USD" {
		t.Fatal("form amount/currency not exact")
	}
	page = guestRequest(t, mux, "GET", path, "", cookie)
	if strings.Contains(page.Body.String(), "<script>window.BAD") || !strings.Contains(page.Body.String(), "&lt;script&gt;") {
		t.Fatal("reason was not escaped")
	}
	approve := order.AfterSaleAction{Key: uuid.Must(uuid.NewV7()).String(), ExpectedVersion: 1, Action: "approve", Message: "Use the return label from support.\nDo not send to the dispatch location."}
	c, err := h.order.ReviewAfterSale(ctx, "private-order", c.PublicID, approve)
	if err != nil {
		t.Fatal(err)
	}
	page = guestRequest(t, mux, "GET", path, "", cookie)
	actionPath := path + "/" + c.PublicID + "/actions"
	if strings.Count(page.Body.String(), "Do not send to the dispatch location.") != 2 {
		t.Fatal("return instructions should appear once in the current message and once in immutable history")
	}
	tracking := saleForm(t, page, actionPath, "return_tracking")
	tracking.Set("carrier", strings.Repeat("c", 129))
	tracking.Set("tracking_number", "RETURN-001")
	bad := guestRequest(t, mux, "POST", actionPath, tracking.Encode(), cookie)
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), "RETURN-001") || saleForm(t, bad, actionPath, "return_tracking").Get("request_key") != tracking.Get("request_key") {
		t.Fatal("invalid tracking lost original input", bad.Code)
	}
	tracking.Set("carrier", "Return carrier")
	for range 2 {
		response := guestRequest(t, mux, "POST", actionPath, tracking.Encode(), cookie)
		if response.Code != 303 {
			t.Fatal("tracking/replay", response.Code)
		}
	}
	salePageCount(t, h, 1, 3)
	tracking.Set("tracking_number", "CHANGED-WITH-SAME-KEY")
	conflict := guestRequest(t, mux, "POST", actionPath, tracking.Encode(), cookie)
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), "CHANGED-WITH-SAME-KEY") || !strings.Contains(conflict.Body.String(), "Review the latest requests") {
		t.Fatal("stale tracking was silently re-bound", conflict.Code)
	}
	salePageCount(t, h, 1, 3)
	c = saleCase(t, h)
	c, err = h.order.ReviewAfterSale(ctx, "private-order", c.PublicID, order.AfterSaleAction{Key: uuid.Must(uuid.NewV7()).String(), ExpectedVersion: c.Version, Action: "receive_return", Message: "Parcel received; inspecting the item."})
	if err != nil {
		t.Fatal(err)
	}
	c, err = h.order.ReviewAfterSale(ctx, "private-order", c.PublicID, order.AfterSaleAction{Key: uuid.Must(uuid.NewV7()).String(), ExpectedVersion: c.Version, Action: "approve", Message: "Inspection passed. Waiting for the actual refund."})
	if err != nil {
		t.Fatal(err)
	}
	page = guestRequest(t, mux, "GET", path, "", cookie)
	for _, want := range []string{"Refund approved — awaiting processing", "actual refund has not yet been recorded as complete", "RETURN-001", "Do not send to the dispatch location"} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("missing return fact %s", want)
		}
	}
	var refunds, outbox int
	if err = h.svc.db.QueryRow(`SELECT (SELECT COUNT(*) FROM refund_request),(SELECT COUNT(*) FROM email_outbox)`).Scan(&refunds, &outbox); err != nil || refunds != 0 || outbox != 2 {
		t.Fatal("review automatically refunded/mailed", err)
	}
	if err = h.order.RevokeAllGuestAccess(ctx, "private-order"); err != nil {
		t.Fatal(err)
	}
	revoked := guestRequest(t, mux, "GET", path, "", cookie)
	assertNoPrivateOrder(t, revoked)
	if strings.Contains(revoked.Body.String(), "RETURN-001") {
		t.Fatal("revoked grant still saw request")
	}
}

func TestAfterSaleFormUsesOrderCurrencyAndNeverReadsQueryDraft(t *testing.T) {
	h, mux, cookie := afterSalePageFixture(t, false)
	path := afterSalePath("private-order")
	for _, q := range []string{`UPDATE "order" SET currency='GBP' WHERE public_id='private-order'`, `UPDATE payment_receipt SET currency='GBP'`} {
		if _, err := h.svc.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	page := guestRequest(t, mux, "GET", path, "", cookie)
	if !strings.Contains(page.Body.String(), "GBP") || !strings.Contains(page.Body.String(), "£123.45") || strings.Contains(page.Body.String(), "$123.45") {
		t.Fatal("request used current market instead of frozen order currency")
	}
	orderPage := guestRequest(t, mux, "GET", guestOrderPath("private-order"), "", cookie)
	if !strings.Contains(orderPage.Body.String(), "£123.45") || !strings.Contains(orderPage.Body.String(), path) {
		t.Fatal("private order currency or entry differs")
	}
	form := saleForm(t, page, path, "")
	form.Set("kind", "refund")
	form.Set("amount", "123.46")
	form.Set("reason", "Keep my typed reason")
	invalid := guestRequest(t, mux, "POST", path, form.Encode(), cookie)
	if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), "123.46") || !strings.Contains(invalid.Body.String(), "Keep my typed reason") {
		t.Fatal("invalid amount lost draft", invalid.Code)
	}
	onlyQuery := guestRequest(t, mux, "POST", path+"?reason=QUERY-PRIVATE-TEXT&amount=1&kind=refund&request_key="+form.Get("request_key"), "", cookie)
	if onlyQuery.Code != 422 || strings.Contains(onlyQuery.Body.String(), "QUERY-PRIVATE-TEXT") {
		t.Fatal("read draft from URL", onlyQuery.Code)
	}
	form.Set("amount", "0.01")
	if response := guestRequest(t, mux, "POST", path, form.Encode(), cookie); response.Code != 303 {
		t.Fatal("positive minimum rejected", response.Code)
	}
	c := saleCase(t, h)
	if c.AmountCents != 1 || c.Currency != "GBP" {
		t.Fatal("frozen money mismatch")
	}
	salePageCount(t, h, 1, 1)
}

func TestAfterSaleUnknownCreateRetainsOriginalRecoveryAndRetry(t *testing.T) {
	h, mux, cookie := afterSalePageFixture(t, false)
	path := afterSalePath("private-order")
	form := saleForm(t, guestRequest(t, mux, "GET", path, "", cookie), path, "")
	form.Set("kind", "refund")
	form.Set("amount", "1.25")
	form.Set("reason", "OWN-DRAFT <textarea> keep me")
	if _, err := h.svc.db.Exec(`CREATE TRIGGER reject_sale BEFORE INSERT ON after_sale_event BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	failed := guestRequest(t, mux, "POST", path, form.Encode(), cookie)
	if failed.Code != 500 || !strings.Contains(failed.Body.String(), "Continue original request") || strings.Contains(failed.Body.String(), "<textarea> keep me") {
		t.Fatal("missing safe recovery", failed.Code)
	}
	recovery := saleForm(t, failed, path, "")
	for _, field := range []string{"request_key", "kind", "amount", "reason"} {
		if recovery.Get(field) != form.Get(field) {
			t.Fatalf("recovery changed %s", field)
		}
	}
	salePageCount(t, h, 0, 0)
	if _, err := h.svc.db.Exec(`DROP TRIGGER reject_sale`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if response := guestRequest(t, mux, "POST", path, recovery.Encode(), cookie); response.Code != 303 {
			t.Fatal("recovery failed", response.Code)
		}
	}
	salePageCount(t, h, 1, 1)
}

func TestAfterSaleCurrentPolicyFailureDoesNotBlockExistingRequest(t *testing.T) {
	h, mux, cookie := afterSalePageFixture(t, false)
	ctx := context.Background()
	c, err := h.order.CreateAfterSale(ctx, "private-order", cookie.Value, order.AfterSaleInput{Key: uuid.Must(uuid.NewV7()).String(), Kind: "refund", AmountCents: 50, Reason: "Existing request remains readable"})
	if err != nil {
		t.Fatal(err)
	}
	if err = h.settings.SetPlain(ctx, "store.policy_profile", "{invalid-profile"); err != nil {
		t.Fatal(err)
	}
	path := afterSalePath("private-order")
	page := guestRequest(t, mux, "GET", path, "", cookie)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Existing request remains readable") || !strings.Contains(page.Body.String(), "policy details are temporarily unavailable") {
		t.Fatal("unrelated policy failure hid the request", page.Code)
	}
	actionPath := path + "/" + c.PublicID + "/actions"
	form := saleForm(t, page, actionPath, "withdraw")
	if response := guestRequest(t, mux, "POST", actionPath, form.Encode(), cookie); response.Code != 303 {
		t.Fatal("policy failure blocked withdrawal", response.Code)
	}
	if saleCase(t, h).State != "withdrawn" {
		t.Fatal("withdrawal missing")
	}
}

func TestAfterSaleReadFailurePreservesOwnPostWithoutStalePrivateRecords(t *testing.T) {
	for _, stage := range []string{"database", "navigation"} {
		t.Run(stage, func(t *testing.T) {
			h, _, cookie := afterSalePageFixture(t, false)
			ctx := context.Background()
			if _, err := h.order.CreateAfterSale(ctx, "private-order", cookie.Value, order.AfterSaleInput{Key: uuid.Must(uuid.NewV7()).String(), Kind: "refund", AmountCents: 50, Reason: "OLDER-RECORD-PRIVATE"}); err != nil {
				t.Fatal(err)
			}
			form := &afterSaleForm{Operation: "request", RequestKey: uuid.Must(uuid.NewV7()).String(), Kind: "refund", Amount: "1.25", Reason: "ONLY-OWN-POST-RECOVERY"}
			r := httptest.NewRequest("POST", afterSalePath("private-order"), nil)
			r.SetPathValue("id", "private-order")
			r.AddCookie(cookie)
			if stage == "database" {
				if err := h.svc.db.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := h.svc.db.Exec(`ALTER TABLE content_page RENAME TO fixture_page_unavailable`); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			httpx.PrivateHeaders(w)
			// Model a read failure after the write result became uncertain; no new
			// authority or case mutation is granted by this isolated view test.
			h.renderAfterSales(w, r, 500, "Continue the original request.", form, true, false)
			requirePrivate(t, w)
			if w.Code != 500 || strings.Contains(w.Body.String(), "OLDER-RECORD-PRIVATE") || !strings.Contains(w.Body.String(), "ONLY-OWN-POST-RECOVERY") {
				t.Fatal("fallback discarded own draft or showed unconfirmed data", w.Code)
			}
			if recovered := saleForm(t, w, afterSalePath("private-order"), ""); recovered.Get("request_key") != form.RequestKey || recovered.Get("reason") != form.Reason {
				t.Fatal("fallback lost original operation")
			}
		})
	}
}

func TestAfterSaleBodyLimitRateLimitAndCustomerCannotApprove(t *testing.T) {
	h, mux, cookie := afterSalePageFixture(t, false)
	path := afterSalePath("private-order")
	r := httptest.NewRequest("POST", path, strings.NewReader("reason="+strings.Repeat("x", commerceMaxBody)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.AddCookie(cookie)
	r.ContentLength = -1
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	requirePrivate(t, w)
	if w.Code != 413 {
		t.Fatal("chunked oversized form", w.Code)
	}
	for range 11 {
		response := guestRequest(t, mux, "POST", path, "amount=0", cookie)
		if response.Code != 422 {
			t.Fatal("unexpected rate counter", response.Code)
		}
	}
	limited := guestRequest(t, mux, "POST", path, "amount=0", cookie)
	if limited.Code != 429 || limited.Header().Get("Retry-After") == "" {
		t.Fatal("unbounded form requests", limited.Code)
	}
	salePageCount(t, h, 0, 0)
	// A separate limiter entry proves the forbidden action is rejected by the
	// operation boundary, rather than merely by the prior IP quota.
	input := order.AfterSaleInput{Key: uuid.Must(uuid.NewV7()).String(), Kind: "refund", Reason: "Need help", AmountCents: 50}
	c, err := h.order.CreateAfterSale(context.Background(), "private-order", cookie.Value, input)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", path+"/"+c.PublicID+"/actions", strings.NewReader(url.Values{"action": {"approve"}, "message": {"Self approval"}, "request_key": {uuid.Must(uuid.NewV7()).String()}, "expected_version": {"1"}}.Encode()))
	req.RemoteAddr = "198.51.100.25:5555"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(cookie)
	denied := httptest.NewRecorder()
	mux.ServeHTTP(denied, req)
	requirePrivate(t, denied)
	if denied.Code != 422 {
		t.Fatal("customer approved own case", denied.Code)
	}
	invalidVersion := httptest.NewRequest("POST", path+"/"+c.PublicID+"/actions", strings.NewReader(url.Values{"action": {"withdraw"}, "request_key": {uuid.Must(uuid.NewV7()).String()}, "expected_version": {"0"}}.Encode()))
	invalidVersion.RemoteAddr = "198.51.100.25:5555"
	invalidVersion.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalidVersion.Header.Set("Sec-Fetch-Site", "same-origin")
	invalidVersion.AddCookie(cookie)
	review := httptest.NewRecorder()
	mux.ServeHTTP(review, invalidVersion)
	requirePrivate(t, review)
	if review.Code != 422 || !strings.Contains(review.Body.String(), "Review the latest requests before making another action") {
		t.Fatal("invalid version did not require a fresh review", review.Code)
	}
	salePageCount(t, h, 1, 1)
}
