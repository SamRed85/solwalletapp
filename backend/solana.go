package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const maxPaymentAge = time.Hour

type rpcTransaction struct {
	BlockTime *int64 `json:"blockTime"`
	Meta      *struct {
		Err any `json:"err"`
	} `json:"meta"`
	Transaction struct {
		Message struct {
			Instructions []struct {
				Program   string          `json:"program"`
				ProgramID string          `json:"programId"`
				Parsed    json.RawMessage `json:"parsed"`
			} `json:"instructions"`
		} `json:"message"`
	} `json:"transaction"`
}

type parsedTransfer struct {
	Type string `json:"type"`
	Info struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
		Lamports    uint64 `json:"lamports"`
	} `json:"info"`
}

type solanaRPC struct {
	url    string
	client *http.Client
}

func newSolanaRPC(url string) *solanaRPC {
	return &solanaRPC{url: url, client: &http.Client{Timeout: 15 * time.Second}}
}

var errRateLimited = errors.New("solana RPC rate limit reached, try again shortly")

// call retries rate-limited requests a few times; the public devnet RPC limits bursts.
func (s *solanaRPC) call(ctx context.Context, method string, params []any, result any) error {
	delay := time.Second
	for attempt := 0; ; attempt++ {
		err := s.callOnce(ctx, method, params, result)
		if !errors.Is(err, errRateLimited) || attempt == 4 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

func (s *solanaRPC) callOnce(ctx context.Context, method string, params []any, result any) error {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("solana RPC unreachable: %s", strings.ReplaceAll(err.Error(), s.url, redactRPC(s.url)))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return errRateLimited
	}

	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("invalid solana RPC response: %w", err)
	}
	if out.Error != nil {
		return fmt.Errorf("solana RPC error: %s", out.Error.Message)
	}
	return json.Unmarshal(out.Result, result)
}

func (s *solanaRPC) getTransaction(ctx context.Context, signature string) (*rpcTransaction, error) {
	var tx *rpcTransaction
	err := s.call(ctx, "getTransaction", []any{signature, map[string]any{
		"encoding":                       "jsonParsed",
		"commitment":                     "finalized",
		"maxSupportedTransactionVersion": 0,
	}}, &tx)
	return tx, err
}

type blockRef struct {
	Slot      uint64
	Blockhash string
	Time      time.Time
}

var finalized = map[string]any{"commitment": "finalized"}

// blockFrom returns the first produced block at or after slot (slots can be skipped).
func (s *solanaRPC) blockFrom(ctx context.Context, slot uint64) (uint64, int64, error) {
	var t *int64
	if err := s.call(ctx, "getBlockTime", []any{slot}, &t); err == nil && t != nil {
		return slot, *t, nil
	} else if errors.Is(err, errRateLimited) || errors.Is(err, context.DeadlineExceeded) {
		return 0, 0, err
	}
	var slots []uint64
	if err := s.call(ctx, "getBlocksWithLimit", []any{slot, 1, finalized}, &slots); err != nil {
		return 0, 0, err
	}
	if len(slots) == 0 {
		return 0, 0, fmt.Errorf("no finalized block at or after slot %d", slot)
	}
	if err := s.call(ctx, "getBlockTime", []any{slots[0]}, &t); err != nil {
		return 0, 0, err
	}
	if t == nil {
		return 0, 0, fmt.Errorf("block %d has no timestamp", slots[0])
	}
	return slots[0], *t, nil
}

// findDrawBlock returns the first finalized block whose timestamp is at or after t.
// Nobody chooses the block, so the draw seed cannot be picked by the admin.
func (s *solanaRPC) findDrawBlock(ctx context.Context, t time.Time) (*blockRef, error) {
	target := t.Unix()
	var latest uint64
	if err := s.call(ctx, "getSlot", []any{finalized}, &latest); err != nil {
		return nil, err
	}
	hi, hiTime, err := s.blockFrom(ctx, latest)
	if err != nil {
		return nil, err
	}
	if hiTime < target {
		return nil, errors.New("the chain has not reached the draw time yet")
	}

	// Find a lower bound before t (devnet makes roughly 2.5-4 slots per second), widening if needed.
	back := uint64(hiTime-target)*5 + 200
	var lo uint64
	for {
		lo = 0
		if back < hi {
			lo = hi - back
		}
		slot, bt, err := s.blockFrom(ctx, lo)
		if err != nil {
			return nil, err
		}
		if bt < target {
			lo = slot
			break
		}
		if lo == 0 {
			hi, lo = slot, slot
			break
		}
		back *= 2
	}

	// Invariant: block lo is before t, block hi is at or after t, and no block exists in [top, hi).
	top := hi
	for lo+1 < top {
		mid := lo + (top-lo)/2
		slot, bt, err := s.blockFrom(ctx, mid)
		if err != nil {
			return nil, err
		}
		switch {
		case slot >= top:
			top = mid
		case bt >= target:
			hi, top = slot, slot
		default:
			lo = slot
		}
	}

	var block struct {
		Blockhash string `json:"blockhash"`
		BlockTime *int64 `json:"blockTime"`
	}
	if err := s.call(ctx, "getBlock", []any{hi, map[string]any{
		"commitment": "finalized", "transactionDetails": "none", "rewards": false, "maxSupportedTransactionVersion": 0,
	}}, &block); err != nil {
		return nil, err
	}
	if block.Blockhash == "" || block.BlockTime == nil {
		return nil, fmt.Errorf("block %d is not available", hi)
	}
	return &blockRef{Slot: hi, Blockhash: block.Blockhash, Time: time.Unix(*block.BlockTime, 0).UTC()}, nil
}

func paymentMemo(eventID string, system []string) string {
	canonical := eventID + "\n" + strings.Join(system, ",")
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("sol-entry-v1:%x", sum)
}

// verifyPayment checks that a finalized transaction moved exactly the expected lamports
// and carries the coupon-bound memo. Reuse of a signature is prevented by a unique index.
func (s *solanaRPC) verifyPayment(ctx context.Context, signature, payer, treasury string, lamports uint64, memo string) error {
	return s.verifyPaymentWithAge(ctx, signature, payer, treasury, lamports, memo, maxPaymentAge)
}

func (s *solanaRPC) verifyRecoveryPayment(ctx context.Context, signature, payer, treasury string, lamports uint64, memo string) error {
	return s.verifyPaymentWithAge(ctx, signature, payer, treasury, lamports, memo, 0)
}

func refundMemo(paymentSignature string) string {
	return "sol-refund-v1:" + paymentSignature
}

func (s *solanaRPC) verifyRefund(ctx context.Context, refundSignature, paymentSignature, treasury, recipient string, lamports uint64) error {
	return s.verifyPaymentWithAge(ctx, refundSignature, treasury, recipient, lamports, refundMemo(paymentSignature), 0)
}

func (s *solanaRPC) verifyPaymentWithAge(ctx context.Context, signature, payer, treasury string, lamports uint64, memo string, maxAge time.Duration) error {
	raw, err := base58Decode(signature)
	if err != nil || len(raw) != 64 {
		return errors.New("invalid transaction signature")
	}
	tx, err := s.getTransaction(ctx, signature)
	if err != nil {
		return err
	}
	return checkPaymentWithAge(tx, payer, treasury, lamports, memo, time.Now(), maxAge)
}

func checkPayment(tx *rpcTransaction, payer, treasury string, lamports uint64, memo string, now time.Time) error {
	return checkPaymentWithAge(tx, payer, treasury, lamports, memo, now, maxPaymentAge)
}

func checkPaymentWithAge(tx *rpcTransaction, payer, treasury string, lamports uint64, memo string, now time.Time, maxAge time.Duration) error {
	if tx == nil {
		return errors.New("transaction not found or not finalized yet")
	}
	if tx.Meta == nil || tx.Meta.Err != nil {
		return errors.New("transaction failed on chain")
	}
	if tx.BlockTime == nil {
		return errors.New("transaction has no block time")
	}
	if maxAge > 0 && now.Sub(time.Unix(*tx.BlockTime, 0)) > maxAge {
		return errors.New("payment is too old to be used for a new entry; request a recovery instead")
	}
	var paid uint64
	memos := 0
	for _, ix := range tx.Transaction.Message.Instructions {
		if ix.Program == "spl-memo" {
			var value string
			if json.Unmarshal(ix.Parsed, &value) == nil && value == memo {
				memos++
			}
			continue
		}
		if ix.Program != "system" {
			continue
		}
		var transfer parsedTransfer
		if json.Unmarshal(ix.Parsed, &transfer) != nil || transfer.Type != "transfer" {
			continue
		}
		if transfer.Info.Source == payer && transfer.Info.Destination == treasury {
			paid += transfer.Info.Lamports
		}
	}
	if paid != lamports {
		return fmt.Errorf("payment mismatch: expected %d lamports from your wallet to the treasury, found %d", lamports, paid)
	}
	if memo != "" && memos != 1 {
		return errors.New("payment memo does not match this event and coupon")
	}
	return nil
}
