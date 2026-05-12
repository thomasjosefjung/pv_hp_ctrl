package myuplink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	oauthTokenURL            = "https://api.myuplink.com/oauth/token"
	apiBaseURL               = "https://api.myuplink.com/v2"
	extraHotWaterParamID     = "25001"
	extraHotWaterTempParamID = "6146"
	domesticWaterTempParamID = "6"
	operationModeParamID     = "17"
	heatingOffsetParamID     = "5001"
	heatingModeParamID       = "5003"
	tokenRefreshLeeway       = 30 * time.Second
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}

type Parameter struct {
	Value float64 `json:"value"`
}

type OperationModeValue float64

const (
	OperationModeHeatingOperation    OperationModeValue = 0
	OperationModeDomesticHotWater    OperationModeValue = 1
	OperationModeSwimmingPool        OperationModeValue = 2
	OperationModeEVUCutOffTime       OperationModeValue = 3
	OperationModeForcedDefrosting    OperationModeValue = 4
	OperationModeNoRequest           OperationModeValue = 5
	OperationModeHeatExtEnergySource OperationModeValue = 6
	OperationModeCoolingMode         OperationModeValue = 7
)

var OperationModeOptions = struct {
	HeatingOperation    OperationModeValue
	DomesticHotWater    OperationModeValue
	SwimmingPool        OperationModeValue
	EVUCutOffTime       OperationModeValue
	ForcedDefrosting    OperationModeValue
	NoRequest           OperationModeValue
	HeatExtEnergySource OperationModeValue
	CoolingMode         OperationModeValue
}{
	HeatingOperation:    OperationModeHeatingOperation,
	DomesticHotWater:    OperationModeDomesticHotWater,
	SwimmingPool:        OperationModeSwimmingPool,
	EVUCutOffTime:       OperationModeEVUCutOffTime,
	ForcedDefrosting:    OperationModeForcedDefrosting,
	NoRequest:           OperationModeNoRequest,
	HeatExtEnergySource: OperationModeHeatExtEnergySource,
	CoolingMode:         OperationModeCoolingMode,
}

type OperationMode struct {
	Value OperationModeValue `json:"value"`
	Text  string             `json:"text"`
}

type HeatingModeValue float64

const (
	HeatingModeAutomatic      HeatingModeValue = 0
	HeatingModeAdditionalHeat HeatingModeValue = 1
	HeatingModeParty          HeatingModeValue = 2
	HeatingModeHoliday        HeatingModeValue = 3
	HeatingModeOff            HeatingModeValue = 4
)

var HeatingModeOptions = struct {
	Automatic      HeatingModeValue
	AdditionalHeat HeatingModeValue
	Party          HeatingModeValue
	Holiday        HeatingModeValue
	Off            HeatingModeValue
}{
	Automatic:      HeatingModeAutomatic,
	AdditionalHeat: HeatingModeAdditionalHeat,
	Party:          HeatingModeParty,
	Holiday:        HeatingModeHoliday,
	Off:            HeatingModeOff,
}

type HeatingMode struct {
	Value HeatingModeValue `json:"value"`
	Text  string           `json:"text"`
}

type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURI  string

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

func NewClient(clientID, clientSecret, redirectURI string) *Client {
	return &Client{
		httpClient:   &http.Client{},
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
	}
}

func (c *Client) fetchAccessToken() (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("scope", "READSYSTEM WRITESYSTEM")

	req, err := http.NewRequest("POST", oauthTokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.httpClient.Do(req)
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

	c.mu.Lock()
	c.accessToken = tokenResponse.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
	c.mu.Unlock()

	return &tokenResponse, nil
}

func (c *Client) ensureAccessToken() error {
	c.mu.Lock()
	hasValidToken := c.accessToken != "" && (c.tokenExpiry.IsZero() || time.Now().Before(c.tokenExpiry.Add(-tokenRefreshLeeway)))
	c.mu.Unlock()
	if hasValidToken {
		return nil
	}

	_, err := c.fetchAccessToken()
	return err
}

func (c *Client) invalidateAccessToken() {
	c.mu.Lock()
	c.accessToken = ""
	c.tokenExpiry = time.Time{}
	c.mu.Unlock()
}

func (c *Client) currentAccessToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.accessToken
}

func (c *Client) doRequest(method, requestURL string, body []byte, contentType string) (*http.Response, error) {
	if err := c.ensureAccessToken(); err != nil {
		return nil, err
	}

	resp, err := c.sendRequest(method, requestURL, body, contentType, c.currentAccessToken())
	if err != nil {
		return nil, err
	}

	if !isInvalidBearerResponse(resp.StatusCode) {
		return resp, nil
	}

	resp.Body.Close()
	c.invalidateAccessToken()

	if err := c.ensureAccessToken(); err != nil {
		return nil, err
	}

	return c.sendRequest(method, requestURL, body, contentType, c.currentAccessToken())
}

func (c *Client) sendRequest(method, requestURL string, body []byte, contentType, accessToken string) (*http.Response, error) {
	var requestBody io.Reader
	if body != nil {
		requestBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, requestURL, requestBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return c.httpClient.Do(req)
}

func isInvalidBearerResponse(statusCode int) bool {
	return statusCode == http.StatusUnauthorized
}

func readResponseError(resp *http.Response, prefix string) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %s", prefix, resp.Status)
	}

	if len(body) == 0 {
		return fmt.Errorf("%s: %s", prefix, resp.Status)
	}

	return fmt.Errorf("%s: %s: %s", prefix, resp.Status, string(body))
}

func (c *Client) SetExtraHotWater(deviceID string, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}

	return c.setDeviceParameter(deviceID, extraHotWaterParamID, value, "failed to update extra hot water")
}

func (c *Client) SetExtraHotWaterTemperature(deviceID string, value float64) error {
	formatted := strconv.FormatFloat(value, 'f', -1, 64)
	return c.setDeviceParameter(deviceID, extraHotWaterTempParamID, formatted, "failed to update extra hot water temperature")
}

func (c *Client) GetExtraHotWater(deviceID string) (bool, error) {
	value, err := c.getDeviceParameter(deviceID, extraHotWaterParamID, "failed to get extra hot water status")
	if err != nil {
		return false, err
	}

	return value != 0.0, nil
}

func (c *Client) GetDomesticWaterTemperature(deviceID string) (float64, error) {
	return c.getDeviceParameter(deviceID, domesticWaterTempParamID, "failed to get domestic hot water temperature")
}

func (c *Client) GetOperationMode(deviceID string) (OperationMode, error) {
	value, err := c.getDeviceParameter(deviceID, operationModeParamID, "failed to get heat pump operation mode")
	if err != nil {
		return OperationMode{}, err
	}

	modeValue := OperationModeValue(value)

	return OperationMode{
		Value: modeValue,
		Text:  operationModeText(modeValue),
	}, nil
}

func (c *Client) SetHeatingTemperatureOffset(deviceID string, value float64) error {
	formatted := strconv.FormatFloat(value, 'f', -1, 64)
	return c.setDeviceParameter(deviceID, heatingOffsetParamID, formatted, "failed to update heating temperature offset")
}

func (c *Client) GetHeatingTemperatureOffset(deviceID string) (float64, error) {
	return c.getDeviceParameter(deviceID, heatingOffsetParamID, "failed to get heating temperature offset")
}

func (c *Client) GetHeatingMode(deviceID string) (HeatingMode, error) {
	value, err := c.getDeviceParameter(deviceID, heatingModeParamID, "failed to get heating operation mode")
	if err != nil {
		return HeatingMode{}, err
	}

	modeValue := HeatingModeValue(value)

	return HeatingMode{
		Value: modeValue,
		Text:  heatingModeText(modeValue),
	}, nil
}

func (c *Client) setDeviceParameter(deviceID, parameterID, value, errorPrefix string) error {
	requestURL := fmt.Sprintf("%s/devices/%s/points", apiBaseURL, deviceID)
	body := []byte(fmt.Sprintf(`{"%s": %q}`, parameterID, value))

	resp, err := c.doRequest("PATCH", requestURL, body, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return readResponseError(resp, errorPrefix)
	}

	return nil
}

func (c *Client) getDeviceParameter(deviceID, parameterID, errorPrefix string) (float64, error) {
	requestURL := fmt.Sprintf("%s/devices/%s/points?parameters=%s", apiBaseURL, deviceID, parameterID)
	resp, err := c.doRequest("GET", requestURL, nil, "application/json")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, readResponseError(resp, errorPrefix)
	}

	var parameters []Parameter
	if err := json.NewDecoder(resp.Body).Decode(&parameters); err != nil {
		return 0, err
	}

	if len(parameters) == 0 {
		return 0, fmt.Errorf("no parameters found in response")
	}

	return parameters[0].Value, nil
}

func operationModeText(value OperationModeValue) string {
	switch value {
	case OperationModeHeatingOperation:
		return "heating operation"
	case OperationModeDomesticHotWater:
		return "domestic hot water"
	case OperationModeSwimmingPool:
		return "swimming pool"
	case OperationModeEVUCutOffTime:
		return "evu cut-off time"
	case OperationModeForcedDefrosting:
		return "forced defrosting"
	case OperationModeNoRequest:
		return "no request"
	case OperationModeHeatExtEnergySource:
		return "heat.ext.energ.source"
	case OperationModeCoolingMode:
		return "cooling mode"
	default:
		return "unknown"
	}
}

func heatingModeText(value HeatingModeValue) string {
	switch value {
	case HeatingModeAutomatic:
		return "automatic"
	case HeatingModeAdditionalHeat:
		return "add. heat gen."
	case HeatingModeParty:
		return "party"
	case HeatingModeHoliday:
		return "holiday"
	case HeatingModeOff:
		return "Off"
	default:
		return "unknown"
	}
}
