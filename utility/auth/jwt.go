package auth

import (
	"SuperBizAgent/utility/common"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const revokedTokenKeyPrefix = "opscaptionai:revoked:"

const (
	defaultTokenExpiry = 24 * time.Hour
	issuer             = "SuperBizAgent"

	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

type Claims struct {
	Sub       string `json:"sub"`
	Role      string `json:"role"`
	Iss       string `json:"iss"`
	Iat       int64  `json:"iat"`
	Exp       int64  `json:"exp"`
	SessionID string `json:"sid,omitempty"`
}

type TokenPair struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

var (
	jwtSecret     []byte
	jwtSecretOnce sync.Once
)

var ErrMissingJWTSecret = fmt.Errorf("auth.jwt_secret is not configured; refusing to start")

func loadSecret() []byte {
	jwtSecretOnce.Do(func() {
		if len(jwtSecret) > 0 {
			return
		}
		v, err := g.Cfg().Get(context.Background(), "auth.jwt_secret")
		resolved := common.ResolveEnv(v.String())
		if err != nil || resolved == "" || common.LooksLikePlaceholderSecret(resolved) {
			panic(ErrMissingJWTSecret)
		}
		jwtSecret = []byte(resolved)
	})
	return jwtSecret
}

func ValidateConfig() error {
	v, err := g.Cfg().Get(context.Background(), "auth.jwt_secret")
	resolved := common.ResolveEnv(v.String())
	if err != nil || resolved == "" || common.LooksLikePlaceholderSecret(resolved) {
		return ErrMissingJWTSecret
	}
	if len(resolved) < 32 {
		return fmt.Errorf("auth.jwt_secret must be at least 32 characters")
	}
	return nil
}

func GenerateToken(userID string, role string) (*TokenPair, error) {
	secret := loadSecret()
	now := time.Now()
	role = NormalizeRole(role)
	if err := ValidateRole(role); err != nil {
		return nil, err
	}

	expiry := defaultTokenExpiry
	v, err := g.Cfg().Get(context.Background(), "auth.token_expiry_hours")
	if err == nil && v.Int64() > 0 {
		expiry = time.Duration(v.Int64()) * time.Hour
	}

	claims := &Claims{
		Sub:  userID,
		Role: role,
		Iss:  issuer,
		Iat:  now.Unix(),
		Exp:  now.Add(expiry).Unix(),
	}

	token, err := encodeJWT(claims, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &TokenPair{
		AccessToken: token,
		ExpiresIn:   int64(expiry.Seconds()),
		TokenType:   "Bearer",
	}, nil
}

func ValidateToken(tokenStr string) (*Claims, error) {
	secret := loadSecret()

	if isTokenRevoked(tokenStr) {
		return nil, fmt.Errorf("token has been revoked")
	}

	claims, err := decodeJWT(tokenStr, secret)
	if err != nil {
		return nil, err
	}

	if claims.Exp < time.Now().Unix() {
		return nil, fmt.Errorf("token expired")
	}

	if claims.Iss != issuer {
		return nil, fmt.Errorf("invalid token issuer")
	}
	if err := ValidateRole(claims.Role); err != nil {
		return nil, err
	}
	claims.Role = NormalizeRole(claims.Role)

	return claims, nil
}

func RevokeToken(tokenStr string) {
	ttl := defaultTokenExpiry
	if secret := loadSecret(); len(secret) > 0 {
		if claims, err := decodeJWT(tokenStr, secret); err == nil && claims.Exp > 0 {
			remaining := time.Until(time.Unix(claims.Exp, 0))
			if remaining > 0 {
				ttl = remaining
			}
		}
	}
	storeRevokedToken(tokenStr, ttl)
}

func revokedTokenKey(tokenStr string) string {
	hash := sha256.Sum256([]byte(tokenStr))
	return revokedTokenKeyPrefix + fmt.Sprintf("%x", hash[:8])
}

func storeRevokedToken(tokenStr string, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := revokedTokenKey(tokenStr)
	if _, err := g.Redis().Do(ctx, "SET", key, "1", "EX", int(ttl.Seconds())); err != nil {
		g.Log().Warningf(ctx, "[auth] failed to persist revoked token to Redis: %v", err)
	}
}

func isTokenRevoked(tokenStr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	key := revokedTokenKey(tokenStr)
	val, err := g.Redis().Do(ctx, "EXISTS", key)
	if err != nil {
		g.Log().Warningf(ctx, "[auth] Redis check for revoked token failed: %v", err)
		return false
	}
	return val.Int64() > 0
}

func encodeJWT(claims *Claims, secret []byte) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64URLEncode(headerJSON)
	claimsB64 := base64URLEncode(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	signature := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

func decodeJWT(tokenStr string, secret []byte) (*Claims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	expectedSig := base64URLEncode(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	claimsJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token payload: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("invalid token claims: %w", err)
	}

	return &claims, nil
}

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func base64URLDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func NormalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return RoleViewer
	}
	return role
}

func ValidateRole(role string) error {
	switch NormalizeRole(role) {
	case RoleAdmin, RoleOperator, RoleViewer:
		return nil
	default:
		return fmt.Errorf("invalid role %q", role)
	}
}

func IsRoleAllowed(role string, allowed ...string) bool {
	role = NormalizeRole(role)
	if role == RoleAdmin {
		return true
	}
	for _, candidate := range allowed {
		if role == NormalizeRole(candidate) {
			return true
		}
	}
	return false
}

func RequiredRolesForPath(path string) []string {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "/api/ai_ops", "/api/ai_ops_trace", "/api/upload", "/api/token_audit", "/api/approval_requests", "/api/memories":
		return []string{RoleOperator, RoleAdmin}
	case "/api/approval_requests/approve", "/api/approval_requests/reject", "/api/memories/action", "/api/memories/promote":
		return []string{RoleAdmin}
	default:
		return nil
	}
}
