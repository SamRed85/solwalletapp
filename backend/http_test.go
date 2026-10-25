package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func testDatabase(t *testing.T) *mongo.Database {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skip(err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Skip(err)
	}
	name := "sol_test_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db := client.Database(name)
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	return db
}

func testApp(t *testing.T, db *mongo.Database) *App {
	t.Helper()
	admin := defaultWallet
	app := &App{
		cfg: Config{
			TreasuryWallet:      admin,
			AdminWallets:        map[string]bool{admin: true},
			ProgramID:           testProgram,
			CouponPriceLamports: 1_000_000,
			SessionSecret:       []byte("test-secret"),
		},
		db:       db,
		nonces:   newNonceStore(db.Collection("nonces")),
		limiter:  rateLimiter{coll: db.Collection("limits")},
		users:    db.Collection("users"),
		events:   db.Collection("events"),
		entries:  db.Collection("entries"),
		payments: db.Collection("payments"),
		rollover: db.Collection("rollover"),
		reveals:  db.Collection("reveals"),
		sessions: db.Collection("sessions"),
		verifyRecoveryPayment: func(context.Context, string, string, string, uint64, string) error {
			return nil
		},
		verifyRefund: func(context.Context, string, string, string, string, uint64) error {
			return nil
		},
	}
	if err := app.ensureIndexes(); err != nil {
		t.Fatal(err)
	}
	return app
}

func loginCookie(t *testing.T, app *App, wallet string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if _, _, err := app.issueSession(rec, context.Background(), wallet, time.Now()); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Value == "" {
		t.Fatalf("session cookie %+v", cookies)
	}
	return cookies[0]
}

func cookieCall(app *App, method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	app.routes().ServeHTTP(rec, req)
	return rec
}

func insertEvent(t *testing.T, app *App, price uint64, vault bool) bson.ObjectID {
	t.Helper()
	event := Event{
		Title:               "recovery",
		GameCount:           1,
		CouponPriceLamports: price,
		Games:               []Game{{HomeTeam: "A", AwayTeam: "B"}},
		StartTime:           time.Now().Add(time.Hour),
		EndTime:             time.Now().Add(2 * time.Hour),
		CreatedAt:           time.Now(),
	}
	if vault {
		event.Chain = &ChainVault{ProgramID: app.cfg.ProgramID, Admin: defaultWallet, Address: defaultWallet, Nonce: 1}
	}
	res, err := app.events.InsertOne(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	return res.InsertedID.(bson.ObjectID)
}

func TestPaymentRecoveryHTTP(t *testing.T) {
	app := testApp(t, testDatabase(t))
	player, _ := testWallet(t)
	playerCookie := loginCookie(t, app, player)
	adminCookie := loginCookie(t, app, defaultWallet)
	eventID := insertEvent(t, app, 1_000_000, false)
	path := "/api/events/" + eventID.Hex() + "/payment-recoveries"

	var seenMemo string
	var seenLamports uint64
	app.verifyRecoveryPayment = func(_ context.Context, signature, payer, treasury string, lamports uint64, memo string) error {
		if signature == "unpaid" {
			return errors.New("payment was not finalized")
		}
		if payer == player && treasury == defaultWallet {
			seenMemo, seenLamports = memo, lamports
		}
		return nil
	}
	app.verifyRefund = func(_ context.Context, refundSignature, paymentSignature, treasury, recipient string, lamports uint64) error {
		if refundSignature == "bad-refund" {
			return errors.New("refund memo does not match")
		}
		if paymentSignature == "pay-1" && (treasury != defaultWallet || recipient != player || lamports != 1_000_000) {
			t.Fatalf("refund check %s %s %s %d", paymentSignature, treasury, recipient, lamports)
		}
		return nil
	}

	if got := cookieCall(app, http.MethodPost, path, `{"system":["1"],"signature":"pay-1"}`, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous status %d", got)
	}
	if got := cookieCall(app, http.MethodPost, path, `{"system":["Y"],"signature":"pay-1"}`, playerCookie).Code; got != http.StatusBadRequest {
		t.Fatalf("bad picks status %d", got)
	}
	if got := cookieCall(app, http.MethodPost, path, `{"system":["1"],"signature":"unpaid"}`, playerCookie).Code; got != http.StatusPaymentRequired {
		t.Fatalf("unpaid status %d", got)
	}

	created := cookieCall(app, http.MethodPost, path, `{"system":["1"],"signature":"pay-1"}`, playerCookie)
	if created.Code != http.StatusOK {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	var payment PaymentRecord
	if err := json.Unmarshal(created.Body.Bytes(), &payment); err != nil {
		t.Fatal(err)
	}
	if payment.Status != paymentRecoveryPending || payment.Lamports != 1_000_000 || seenLamports != 1_000_000 {
		t.Fatalf("payment %+v seen %d", payment, seenLamports)
	}
	if seenMemo != paymentMemo(eventID.Hex(), []string{"1"}) {
		t.Fatalf("memo %q", seenMemo)
	}

	again := cookieCall(app, http.MethodPost, path, `{"system":["1"],"signature":"pay-1"}`, playerCookie)
	if again.Code != http.StatusOK {
		t.Fatalf("repeat %d %s", again.Code, again.Body.String())
	}
	other, _ := testWallet(t)
	if got := cookieCall(app, http.MethodPost, path, `{"system":["1"],"signature":"pay-1"}`, loginCookie(t, app, other)).Code; got != http.StatusConflict {
		t.Fatalf("other wallet status %d", got)
	}
	if got := cookieCall(app, http.MethodGet, "/api/payment-recoveries", "", playerCookie).Code; got != http.StatusForbidden {
		t.Fatalf("player list status %d", got)
	}

	listed := cookieCall(app, http.MethodGet, "/api/payment-recoveries", "", adminCookie)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "pay-1") {
		t.Fatalf("list %d %s", listed.Code, listed.Body.String())
	}

	rejected := cookieCall(app, http.MethodPut, "/api/payment-recoveries/pay-1", `{"refundSignature":"bad-refund"}`, adminCookie)
	if rejected.Code != http.StatusPaymentRequired {
		t.Fatalf("bad refund %d %s", rejected.Code, rejected.Body.String())
	}
	refunded := cookieCall(app, http.MethodPut, "/api/payment-recoveries/pay-1", `{"refundSignature":"refund-1"}`, adminCookie)
	if refunded.Code != http.StatusOK || !strings.Contains(refunded.Body.String(), `"status":"refunded"`) {
		t.Fatalf("refund %d %s", refunded.Code, refunded.Body.String())
	}
	if got := cookieCall(app, http.MethodPut, "/api/payment-recoveries/pay-1", `{"refundSignature":"refund-2"}`, adminCookie).Code; got != http.StatusNotFound {
		t.Fatalf("second refund status %d", got)
	}

	secondID := insertEvent(t, app, 1_000_000, false)
	secondPath := "/api/events/" + secondID.Hex() + "/payment-recoveries"
	if got := cookieCall(app, http.MethodPost, secondPath, `{"system":["1"],"signature":"pay-2"}`, playerCookie).Code; got != http.StatusOK {
		t.Fatalf("second recovery status %d", got)
	}
	if _, err := app.payments.UpdateByID(context.Background(), "pay-1", bson.M{"$set": bson.M{"refundSignature": "refund-shared"}}); err != nil {
		t.Fatal(err)
	}
	shared := cookieCall(app, http.MethodPut, "/api/payment-recoveries/pay-2", `{"refundSignature":"refund-shared"}`, adminCookie)
	if shared.Code != http.StatusConflict {
		t.Fatalf("reused refund signature %d %s", shared.Code, shared.Body.String())
	}
}

func TestVaultRecoveryStaysRefundOnly(t *testing.T) {
	app := testApp(t, testDatabase(t))
	player, _ := testWallet(t)
	eventID := insertEvent(t, app, 1_000_000, true)
	res := cookieCall(app, http.MethodPost, "/api/events/"+eventID.Hex()+"/payment-recoveries", `{"system":["1"],"signature":"vault-pay"}`, loginCookie(t, app, player))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "refund only") {
		t.Fatalf("vault recovery %d %s", res.Code, res.Body.String())
	}
}

func TestRecoveryRejectsAcceptedEntry(t *testing.T) {
	app := testApp(t, testDatabase(t))
	player, _ := testWallet(t)
	eventID := insertEvent(t, app, 1_000_000, false)
	if _, err := app.entries.InsertOne(context.Background(), bson.M{"signature": "used", "eventId": eventID, "walletAddress": player}); err != nil {
		t.Fatal(err)
	}
	res := cookieCall(app, http.MethodPost, "/api/events/"+eventID.Hex()+"/payment-recoveries", `{"system":["1"],"signature":"used"}`, loginCookie(t, app, player))
	if res.Code != http.StatusConflict {
		t.Fatalf("accepted payment status %d %s", res.Code, res.Body.String())
	}
}

func TestSharedNonceAndRateLimit(t *testing.T) {
	db := testDatabase(t)
	first := newNonceStore(db.Collection("nonces"))
	second := newNonceStore(db.Collection("nonces"))
	wallet, _ := testWallet(t)
	now := time.Now()
	nonce, _, err := first.issue(context.Background(), wallet, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := second.consume(context.Background(), nonce, wallet, now); !ok {
		t.Fatal("a second process could not use the nonce")
	}
	if _, ok := first.consume(context.Background(), nonce, wallet, now); ok {
		t.Fatal("nonce was used twice across processes")
	}

	limitA := rateLimiter{coll: db.Collection("limits")}
	limitB := rateLimiter{coll: db.Collection("limits")}
	if !limitA.allow("shared", 2, time.Minute, now) || !limitB.allow("shared", 2, time.Minute, now) {
		t.Fatal("shared window rejected an allowed request")
	}
	if limitA.allow("shared", 2, time.Minute, now) {
		t.Fatal("the second process did not see the first process count")
	}
}

func TestSessionCookieSurvivesASecondProcess(t *testing.T) {
	db := testDatabase(t)
	app := testApp(t, db)
	other := testApp(t, db)
	wallet, priv := testWallet(t)

	nonceRes := cookieCall(app, http.MethodPost, "/api/auth/nonce", `{"publicKey":"`+wallet+`"}`, nil)
	if nonceRes.Code != http.StatusOK {
		t.Fatalf("nonce %d %s", nonceRes.Code, nonceRes.Body.String())
	}
	var issued struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(nonceRes.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(issued.Message)))
	body, _ := json.Marshal(map[string]string{"publicKey": wallet, "nonce": issued.Nonce, "signature": signature})
	login := cookieCall(app, http.MethodPost, "/api/auth/login", string(body), nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login %d %s", login.Code, login.Body.String())
	}
	if bytes.Contains(login.Body.Bytes(), []byte(`"token"`)) {
		t.Fatalf("login returned a page-readable token: %s", login.Body.String())
	}
	cookie := login.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != sessionCookie || !cookie[0].HttpOnly {
		t.Fatalf("login cookie %+v", cookie)
	}

	me := cookieCall(other, http.MethodGet, "/api/me", "", cookie[0])
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), wallet) {
		t.Fatalf("second process me %d %s", me.Code, me.Body.String())
	}
	if got := cookieCall(app, http.MethodPost, "/api/auth/logout", "", cookie[0]).Code; got != http.StatusNoContent {
		t.Fatalf("logout %d", got)
	}
	if got := cookieCall(other, http.MethodGet, "/api/me", "", cookie[0]).Code; got != http.StatusUnauthorized {
		t.Fatalf("revoked session status %d", got)
	}
}

func TestRolloverAppliesOnceAndRepairFinishesIt(t *testing.T) {
	app := testApp(t, testDatabase(t))
	ctx := context.Background()
	now := time.Now()
	targetRes, err := app.events.InsertOne(ctx, Event{Title: "next", StartTime: now.Add(2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	targetID := targetRes.InsertedID.(bson.ObjectID)
	sourceRes, err := app.events.InsertOne(ctx, Event{
		Title: "settled", StartTime: now.Add(-time.Hour), ResultsSetAt: &now,
		Settlement: &Settlement{RolloverOutLamports: 500, RolloverToEventID: &targetID, RolloverApplied: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := sourceRes.InsertedID.(bson.ObjectID)

	app.repairRollover(ctx)
	app.repairRollover(ctx)
	var target Event
	if err := app.events.FindOne(ctx, bson.M{"_id": targetID}).Decode(&target); err != nil {
		t.Fatal(err)
	}
	if target.RolloverInLamports != 500 {
		t.Fatalf("rollover in = %d, want 500", target.RolloverInLamports)
	}
	var source Event
	if err := app.events.FindOne(ctx, bson.M{"_id": sourceID}).Decode(&source); err != nil {
		t.Fatal(err)
	}
	if source.Settlement == nil || !source.Settlement.RolloverApplied {
		t.Fatal("repair did not mark the rollover applied")
	}

	oldID := bson.NewObjectID()
	if _, err := app.events.InsertOne(ctx, bson.M{
		"_id": oldID, "title": "historical", "resultsSetAt": now,
		"settlement": bson.M{"rolloverOutLamports": 900, "rolloverToEventId": targetID},
	}); err != nil {
		t.Fatal(err)
	}
	app.repairRollover(ctx)
	if err := app.events.FindOne(ctx, bson.M{"_id": targetID}).Decode(&target); err != nil {
		t.Fatal(err)
	}
	if target.RolloverInLamports != 500 {
		t.Fatalf("historical settlement was applied again: %d", target.RolloverInLamports)
	}

	pendingSource := bson.NewObjectID()
	if err := app.applyRolloverFrom(ctx, pendingSource, nil, 40); err != nil {
		t.Fatal(err)
	}
	if err := app.applyRolloverFrom(ctx, pendingSource, nil, 40); err != nil {
		t.Fatal(err)
	}
	var pending struct {
		Lamports uint64 `bson:"lamports"`
	}
	if err := app.rollover.FindOne(ctx, bson.M{"_id": pendingRolloverID}).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if pending.Lamports != 40 {
		t.Fatalf("pending rollover = %d, want 40", pending.Lamports)
	}
}
