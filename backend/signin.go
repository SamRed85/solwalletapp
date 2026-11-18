package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxSignedMessage = 4096
	solanaDevnet     = "solana:devnet"
)

// \xffsolana offchain. No valid transaction starts with these bytes.
var offchainPrefix = []byte{0xff, 's', 'o', 'l', 'a', 'n', 'a', ' ', 'o', 'f', 'f', 'c', 'h', 'a', 'i', 'n'}

func decodeSignedMessage(raw string) ([]byte, error) {
	if raw == "" {
		return nil, nil
	}
	signed, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(signed) == 0 || len(signed) > maxSignedMessage {
		return nil, errors.New("signed message must be base64")
	}
	return signed, nil
}

func verifyWalletBytes(wallet string, message, signature []byte) bool {
	pub, err := base58Decode(wallet)
	if err != nil || len(pub) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), message, signature)
}

// loginSignatureAccepted accepts a signature for any origin in ALLOWED_ORIGIN.
func (a *App) loginSignatureAccepted(wallet, stored, nonce string, signature, signed []byte, now time.Time) bool {
	for _, origin := range a.cfg.AllowedOrigins {
		if loginSignatureOK(origin, wallet, stored, nonce, signature, signed, now) {
			return true
		}
	}
	return false
}

// loginSignatureOK accepts the stored sign-in message, the same message inside an off-chain
// wrapper, or a Sign In With Solana message for this origin, wallet, devnet, and nonce.
func loginSignatureOK(origin, wallet, stored, nonce string, signature, signed []byte, now time.Time) bool {
	if len(signature) != ed25519.SignatureSize {
		return false
	}
	if signed == nil {
		return verifyWalletSignature(wallet, stored, signature)
	}
	if !verifyWalletBytes(wallet, signed, signature) {
		return false
	}
	text, ok := signedText(wallet, signed)
	if !ok {
		return false
	}
	if text == stored {
		return true
	}
	return signInTextOK(origin, wallet, nonce, text, now)
}

func signedText(wallet string, signed []byte) (string, bool) {
	if bytes.HasPrefix(signed, offchainPrefix) {
		return unwrapOffchain(wallet, signed)
	}
	if len(signed) == 0 || len(signed) > maxSignedMessage || !utf8.Valid(signed) {
		return "", false
	}
	return string(signed), true
}

func unwrapOffchain(wallet string, signed []byte) (string, bool) {
	if len(signed) < len(offchainPrefix)+2 || !bytes.Equal(signed[:len(offchainPrefix)], offchainPrefix) {
		return "", false
	}
	switch signed[len(offchainPrefix)] {
	case 0:
		return unwrapOffchainV0(wallet, signed)
	case 1:
		return unwrapOffchainV1(wallet, signed)
	default:
		return "", false
	}
}

func unwrapOffchainV1(wallet string, signed []byte) (string, bool) {
	const header = 16 + 1 + 1
	if len(signed) < header+32+1 || signed[17] != 1 {
		return "", false
	}
	if !signerIs(wallet, signed[18:50]) {
		return "", false
	}
	content := signed[50:]
	if len(content) == 0 || !utf8.Valid(content) {
		return "", false
	}
	return string(content), true
}

func unwrapOffchainV0(wallet string, signed []byte) (string, bool) {
	// domain, version, application domain, format, one signer, then the body length.
	const lengthAt = 16 + 1 + 32 + 1 + 1 + 32
	if len(signed) < lengthAt+2+1 || signed[50] != 1 {
		return "", false
	}
	format := signed[49]
	if format > 2 || !signerIs(wallet, signed[51:83]) {
		return "", false
	}
	length := int(binary.LittleEndian.Uint16(signed[lengthAt : lengthAt+2]))
	body := signed[lengthAt+2:]
	if length == 0 || len(body) != length {
		return "", false
	}
	switch format {
	case 0:
		if len(signed) > 1232 || !asciiPrintable(body) {
			return "", false
		}
	case 1:
		if len(signed) > 1232 || !utf8.Valid(body) {
			return "", false
		}
	default:
		if !utf8.Valid(body) {
			return "", false
		}
	}
	return string(body), true
}

func asciiPrintable(body []byte) bool {
	for _, b := range body {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

func signerIs(wallet string, raw []byte) bool {
	pub, err := base58Decode(wallet)
	return err == nil && bytes.Equal(pub, raw)
}

type signInFields struct {
	domain, address, uri, version, chain, nonce, issuedAt, expiration, notBefore string
}

func signInTextOK(origin, wallet, nonce, text string, now time.Time) bool {
	fields, ok := parseSignIn(text)
	if !ok || !isValidWallet(wallet) || nonce == "" {
		return false
	}
	wantOrigin, host, ok := siteOrigin(origin)
	if !ok || fields.domain != host || fields.address != wallet || fields.chain != solanaDevnet || fields.nonce != nonce {
		return false
	}
	if fields.version != "1" || !sameOrigin(fields.uri, wantOrigin) || !issuedFresh(fields.issuedAt, now) {
		return false
	}
	if fields.expiration != "" && !timeAfter(fields.expiration, now) {
		return false
	}
	if fields.notBefore != "" && !notBeforeOK(fields.notBefore, now) {
		return false
	}
	return true
}

func parseSignIn(text string) (signInFields, bool) {
	if strings.Contains(text, "\r") || len(text) > maxSignedMessage {
		return signInFields{}, false
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 4 {
		return signInFields{}, false
	}
	const suffix = " wants you to sign in with your Solana account:"
	domain, ok := strings.CutSuffix(lines[0], suffix)
	if !ok || domain == "" || strings.ContainsAny(domain, " \t/@") {
		return signInFields{}, false
	}
	if lines[1] == "" || lines[2] != "" {
		return signInFields{}, false
	}
	body := lines[3:]
	fieldLines, ok := signInFieldLines(body)
	if !ok {
		return signInFields{}, false
	}
	parsed, ok := parseFieldLines(fieldLines)
	if !ok {
		return signInFields{}, false
	}
	return signInFields{
		domain:     domain,
		address:    lines[1],
		uri:        parsed["URI"],
		version:    parsed["Version"],
		chain:      parsed["Chain ID"],
		nonce:      parsed["Nonce"],
		issuedAt:   parsed["Issued At"],
		expiration: parsed["Expiration Time"],
		notBefore:  parsed["Not Before"],
	}, true
}

func signInFieldLines(body []string) ([]string, bool) {
	if len(body) > 0 && isFieldLine(body[0]) && allFieldLines(body) {
		return body, true
	}
	end := -1
	for i := 0; i < len(body); i++ {
		if body[i] == "" && allFieldLines(body[i+1:]) {
			end = i
		}
	}
	if end < 0 {
		return nil, false
	}
	return body[end+1:], true
}

func allFieldLines(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !isFieldLine(line) {
			return false
		}
	}
	return true
}

func isFieldLine(line string) bool {
	if line == "Resources:" || (strings.HasPrefix(line, "- ") && len(line) > 2) {
		return true
	}
	name, value, ok := strings.Cut(line, ": ")
	return ok && value != "" && knownSignInField(name)
}

func knownSignInField(name string) bool {
	switch name {
	case "URI", "Version", "Chain ID", "Nonce", "Issued At", "Expiration Time", "Not Before", "Request ID":
		return true
	default:
		return false
	}
}

func parseFieldLines(lines []string) (map[string]string, bool) {
	out := map[string]string{}
	resources := false
	for _, line := range lines {
		if resources {
			if !strings.HasPrefix(line, "- ") || strings.TrimPrefix(line, "- ") == "" {
				return nil, false
			}
			continue
		}
		if line == "Resources:" {
			resources = true
			continue
		}
		if strings.HasPrefix(line, "- ") {
			return nil, false
		}
		name, value, ok := strings.Cut(line, ": ")
		if !ok || value == "" || !knownSignInField(name) || out[name] != "" {
			return nil, false
		}
		out[name] = value
	}
	return out, true
}

func siteOrigin(raw string) (origin, host string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", "", false
	}
	return u.Scheme + "://" + u.Host, u.Host, true
}

func sameOrigin(raw, want string) bool {
	got, _, ok := siteOrigin(raw)
	return ok && got == want
}

func parseTime(raw string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, raw)
	return t, err == nil
}

func issuedFresh(raw string, now time.Time) bool {
	t, ok := parseTime(raw)
	if !ok || t.After(now.Add(2*time.Minute)) {
		return false
	}
	return now.Sub(t) <= nonceTTL
}

func timeAfter(raw string, now time.Time) bool {
	t, ok := parseTime(raw)
	return ok && t.After(now)
}

func notBeforeOK(raw string, now time.Time) bool {
	t, ok := parseTime(raw)
	return ok && !t.After(now.Add(2*time.Minute))
}
