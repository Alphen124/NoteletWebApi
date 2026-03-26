package firebase

import (
	"context"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

var client *auth.Client

// Init initialises the Firebase Admin SDK.
// It tries to load credentials from the file named by FIREBASE_SERVICE_ACCOUNT_KEY
// (defaults to "serviceAccountKey.json"). If the file does not exist it falls
// back to Application Default Credentials (useful on GCP / Cloud Run).
func Init() error {
	ctx := context.Background()

	keyFile := os.Getenv("FIREBASE_SERVICE_ACCOUNT_KEY")
	if keyFile == "" {
		keyFile = "serviceAccountKey.json"
	}

	var (
		app *firebase.App
		err error
	)

	if _, statErr := os.Stat(keyFile); statErr == nil {
		app, err = firebase.NewApp(ctx, nil, option.WithCredentialsFile(keyFile))
	} else {
		// Fall back to ADC (Application Default Credentials)
		app, err = firebase.NewApp(ctx, nil)
	}
	if err != nil {
		return fmt.Errorf("firebase.NewApp: %w", err)
	}

	client, err = app.Auth(ctx)
	if err != nil {
		return fmt.Errorf("firebase app.Auth: %w", err)
	}
	return nil
}

// VerifyIDToken verifies a Firebase ID token and returns the decoded token.
func VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	return client.VerifyIDToken(ctx, idToken)
}
