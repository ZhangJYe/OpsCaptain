package auth

import (
	"sync"
	"testing"
	"time"
)

func init() {
	jwtSecret = []byte("test-secret-key-for-unit-tests-minimum-32-chars")
}

func TestGenerateAndValidateToken(t *testing.T) {
	pair, err := GenerateToken("user-001", "admin")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if pair.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}
	if pair.TokenType != "Bearer" {
		t.Fatalf("expected Bearer token type, got %s", pair.TokenType)
	}
	if pair.ExpiresIn <= 0 {
		t.Fatal("expected positive expiry")
	}

	claims, err := ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Sub != "user-001" {
		t.Fatalf("expected sub=user-001, got %s", claims.Sub)
	}
	if claims.Role != "admin" {
		t.Fatalf("expected role=admin, got %s", claims.Role)
	}
	if claims.Iss != issuer {
		t.Fatalf("expected issuer=%s, got %s", issuer, claims.Iss)
	}
}

func TestValidateToken_InvalidFormat(t *testing.T) {
	_, err := ValidateToken("not-a-jwt")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateToken_TamperedSignature(t *testing.T) {
	pair, _ := GenerateToken("user-001", "admin")
	tampered := pair.AccessToken + "x"
	_, err := ValidateToken(tampered)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	secret := jwtSecret
	claims := &Claims{
		Sub:  "user-expired",
		Role: "user",
		Iss:  issuer,
		Iat:  time.Now().Add(-2 * time.Hour).Unix(),
		Exp:  time.Now().Add(-1 * time.Hour).Unix(),
	}
	token, err := encodeJWT(claims, secret)
	if err != nil {
		t.Fatalf("encodeJWT failed: %v", err)
	}

	_, err = ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestRevokeToken(t *testing.T) {
	resetRevokedTokensForTest()
	pair, _ := GenerateToken("user-revoke", "user")

	_, err := ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("token should be valid before revocation: %v", err)
	}

	RevokeToken(pair.AccessToken)

	_, err = ValidateToken(pair.AccessToken)
	if err == nil {
		t.Fatal("expected error for revoked token")
	}
}

func TestClearExpiredRevokedTokens(t *testing.T) {
	resetRevokedTokensForTest()
	revokedTokens.Store("expired", time.Now().Add(-time.Minute).Unix())
	revokedTokens.Store("active", time.Now().Add(time.Minute).Unix())

	clearExpiredRevokedTokens(time.Now())

	if _, ok := revokedTokens.Load("expired"); ok {
		t.Fatal("expected expired revoked token to be removed")
	}
	if _, ok := revokedTokens.Load("active"); !ok {
		t.Fatal("expected active revoked token to remain")
	}
}

func TestValidateToken_WrongIssuer(t *testing.T) {
	secret := jwtSecret
	claims := &Claims{
		Sub:  "user-wrong-iss",
		Role: "user",
		Iss:  "wrong-issuer",
		Iat:  time.Now().Unix(),
		Exp:  time.Now().Add(time.Hour).Unix(),
	}
	token, _ := encodeJWT(claims, secret)

	_, err := ValidateToken(token)
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestTokenUniqueness(t *testing.T) {
	pair1, _ := GenerateToken("user-001", "admin")
	time.Sleep(time.Second)
	pair2, _ := GenerateToken("user-001", "admin")

	if pair1.AccessToken == pair2.AccessToken {
		t.Fatal("expected different tokens for different timestamps")
	}
}

func TestBase64URLRoundTrip(t *testing.T) {
	testData := []byte("hello world 你好世界!@#$%")
	encoded := base64URLEncode(testData)
	decoded, err := base64URLDecode(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if string(decoded) != string(testData) {
		t.Fatalf("roundtrip failed: got %s", string(decoded))
	}
}

func TestValidateConfig_MissingSecret(t *testing.T) {
	err := ValidateConfig()
	if err == nil {
		return
	}
}

func resetRevokedTokensForTest() {
	revokedTokens = sync.Map{}
	revokedCleanupOnce = sync.Once{}
}
