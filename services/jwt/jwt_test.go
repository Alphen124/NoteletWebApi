package jwt

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setup() {
	os.Setenv("JWT_ACCESS_SECRET", "test_access_secret")
	os.Setenv("JWT_REFRESH_SECRET", "test_refresh_secret")
}

func TestGenerateAccessToken(t *testing.T) {
	setup()

	token, err := GenerateAccessToken(1, "test@kmitl.ac.th", false, false, false)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestValidateAccessToken(t *testing.T) {
	setup()

	token, _ := GenerateAccessToken(1, "test@kmitl.ac.th", false, false, false)
	claims, err := ValidateAccessToken(token)

	assert.NoError(t, err)
	assert.Equal(t, 1, claims.UserId)
	assert.Equal(t, "test@kmitl.ac.th", claims.Email)
	assert.False(t, claims.IsAdmin)
}

func TestValidateAccessToken_InvalidToken(t *testing.T) {
	setup()

	_, err := ValidateAccessToken("invalid.token.string")

	assert.Error(t, err)
}
