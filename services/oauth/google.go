package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	// GoogleOAuthConfig ตั้งค่า OAuth2 สำหรับ Google
	// ในการใช้งานจริง ควรเก็บ ClientID และ ClientSecret ใน environment variables
	GoogleOAuthConfig *oauth2.Config
)

// InitGoogleOAuth ตั้งค่า Google OAuth
func InitGoogleOAuth(clientID, clientSecret, redirectURL string) {
	GoogleOAuthConfig = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

// GoogleUserInfo โครงสร้างข้อมูลผู้ใช้จาก Google
type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
	HD            string `json:"hd"` // Hosted domain (e.g., kmitl.ac.th)
}

// GetGoogleUserInfo ดึงข้อมูลผู้ใช้จาก Google โดยใช้ access token
func GetGoogleUserInfo(accessToken string) (*GoogleUserInfo, error) {
	// เรียก Google UserInfo API
	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google api error: %s", string(body))
	}

	// Parse response
	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %v", err)
	}

	return &userInfo, nil
}

// ExchangeCodeForToken แลกเปลี่ยน authorization code เป็น access token
func ExchangeCodeForToken(code string) (*oauth2.Token, error) {
	if GoogleOAuthConfig == nil {
		return nil, fmt.Errorf("google oauth config not initialized")
	}

	token, err := GoogleOAuthConfig.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %v", err)
	}

	return token, nil
}

// GetAuthURL สร้าง URL สำหรับ Google OAuth authorization
func GetAuthURL(state string) string {
	if GoogleOAuthConfig == nil {
		return ""
	}
	return GoogleOAuthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
}
