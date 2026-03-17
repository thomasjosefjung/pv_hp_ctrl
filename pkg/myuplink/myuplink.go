package myuplink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

func GetAccessToken(clientID, clientSecret, redirectURI, authCode string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("scope", "READSYSTEM WRITESYSTEM")
	// data.Set("code", authCode)
	// data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://api.myuplink.com/oauth/token", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get access token: %s", resp.Status)
	}

	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return nil, err
	}

	return &tokenResponse, nil
}

func SetExtraWarmWater(deviceID, accessToken string) error {
	// This is a placeholder for the actual API call to set the extra warm water
	fmt.Printf("Setting extra warm water for device %s with access token %s\n", deviceID, accessToken)

	body := []byte(`{"25001": "0"}`)

	req, err := http.NewRequest(
		"PATCH",
		"https://api.myuplink.com/v2/devices/hp24-r-20251115-04-d1-6e-d9-7b-f8/points",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Content-Type", "application/json")

	// Request ausführen
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
