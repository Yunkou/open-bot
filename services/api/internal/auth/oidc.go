package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Casdoor / generic OIDC settings for open-bot.
// When Endpoint+ClientID+ClientSecret are set, OIDC login is enabled.
type OIDCConfig struct {
	Enabled      bool
	Endpoint     string // e.g. http://127.0.0.1:8000
	ClientID     string
	ClientSecret string
	RedirectURI  string
	OrgName      string // Casdoor organization, default built-in
	AppName      string // Casdoor application, default app-built-in
	Scopes       string
}

func LoadOIDCConfig() OIDCConfig {
	endpoint := strings.TrimRight(firstNonEmpty(
		os.Getenv("CASDOOR_ENDPOINT"),
		os.Getenv("OIDC_ISSUER"),
		os.Getenv("OIDC_ENDPOINT"),
	), "/")
	clientID := firstNonEmpty(os.Getenv("CASDOOR_CLIENT_ID"), os.Getenv("OIDC_CLIENT_ID"))
	clientSecret := firstNonEmpty(os.Getenv("CASDOOR_CLIENT_SECRET"), os.Getenv("OIDC_CLIENT_SECRET"))
	redirect := firstNonEmpty(
		os.Getenv("CASDOOR_REDIRECT_URI"),
		os.Getenv("OIDC_REDIRECT_URI"),
		"http://127.0.0.1:5173/auth/callback",
	)
	org := firstNonEmpty(os.Getenv("CASDOOR_ORGANIZATION"), os.Getenv("CASDOOR_ORG"), "built-in")
	app := firstNonEmpty(os.Getenv("CASDOOR_APPLICATION"), os.Getenv("CASDOOR_APP"), "app-built-in")
	scopes := firstNonEmpty(os.Getenv("CASDOOR_SCOPES"), os.Getenv("OIDC_SCOPES"), "openid profile email")
	cfg := OIDCConfig{
		Endpoint:     endpoint,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirect,
		OrgName:      org,
		AppName:      app,
		Scopes:       scopes,
	}
	cfg.Enabled = endpoint != "" && clientID != "" && clientSecret != ""
	return cfg
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func (c OIDCConfig) AuthorizeURL(state string) string {
	if !c.Enabled {
		return ""
	}
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", c.RedirectURI)
	q.Set("scope", c.Scopes)
	q.Set("state", state)
	// Casdoor accepts organization/application hints
	if c.OrgName != "" {
		q.Set("organization", c.OrgName)
	}
	if c.AppName != "" {
		q.Set("application", c.AppName)
	}
	return c.Endpoint + "/login/oauth/authorize?" + q.Encode()
}

type OIDCTokenResult struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type OIDCUserInfo struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	DisplayName       string `json:"displayName"`
	Username          string // normalized
}

func (c OIDCConfig) ExchangeCode(ctx context.Context, code string) (*OIDCTokenResult, error) {
	if !c.Enabled {
		return nil, errors.New("oidc not configured")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, errors.New("code required")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", c.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+"/api/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("token exchange failed: %s", strings.TrimSpace(string(body)))
	}
	var tok OIDCTokenResult
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, err
	}
	if tok.AccessToken == "" {
		return nil, errors.New("no access_token in response")
	}
	return &tok, nil
}

func (c OIDCConfig) FetchUserInfo(ctx context.Context, accessToken string) (*OIDCUserInfo, error) {
	if !c.Enabled {
		return nil, errors.New("oidc not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+"/api/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("userinfo failed: %s", strings.TrimSpace(string(body)))
	}
	var info OIDCUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	// Casdoor sometimes nests differently; also try raw map fallbacks
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	if info.Sub == "" {
		info.Sub = stringFromAny(raw["sub"], raw["id"])
	}
	if info.PreferredUsername == "" {
		info.PreferredUsername = stringFromAny(raw["preferred_username"], raw["name"], raw["displayName"])
	}
	if info.Email == "" {
		info.Email = stringFromAny(raw["email"])
	}
	info.Username = strings.TrimSpace(info.PreferredUsername)
	if info.Username == "" {
		info.Username = strings.TrimSpace(info.Name)
	}
	if info.Username == "" && info.Email != "" {
		info.Username = strings.Split(info.Email, "@")[0]
	}
	if info.Username == "" && info.Sub != "" {
		info.Username = "casdoor_" + info.Sub
	}
	if info.Sub == "" {
		return nil, errors.New("userinfo missing sub")
	}
	if info.Username == "" {
		return nil, errors.New("userinfo missing username")
	}
	// sanitize username for local unique constraint
	info.Username = sanitizeUsername(info.Username)
	return &info, nil
}

func stringFromAny(vals ...any) string {
	for _, v := range vals {
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		case float64:
			return fmt.Sprintf("%.0f", t)
		}
	}
	return ""
}

func sanitizeUsername(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}
