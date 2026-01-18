package devcycle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/open-feature/cli/internal/logger"
)

const (
	// DevCycle API base URLs
	authURL = "https://auth.devcycle.com/oauth/token"
	apiURL  = "https://api.devcycle.com"
)

// Client is a DevCycle Management API client
type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	accessToken  string
	tokenExpiry  time.Time
	tokenMu      sync.RWMutex
}

// Variable represents a DevCycle variable
type Variable struct {
	ID           string `json:"_id,omitempty"`
	Key          string `json:"key"`
	Name         string `json:"name,omitempty"`
	Type         string `json:"type"` // Boolean, String, Number, JSON
	Description  string `json:"description,omitempty"`
	DefaultValue any    `json:"defaultValue,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

// Feature represents a DevCycle feature
type Feature struct {
	ID          string     `json:"_id,omitempty"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Type        string     `json:"type,omitempty"` // release, experiment, permission, ops
	Variables   []Variable `json:"variables,omitempty"`
	Variations  []any      `json:"variations,omitempty"`
}

// tokenResponse represents the OAuth token response
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// NewClient creates a new DevCycle API client
func NewClient(clientID, clientSecret string) (*Client, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("clientId and clientSecret are required")
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		clientID:     clientID,
		clientSecret: clientSecret,
	}, nil
}

// authenticate obtains or refreshes the OAuth access token
func (c *Client) authenticate(ctx context.Context) error {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Check if we have a valid token
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	logger.Default.Debug("DevCycle: Authenticating with OAuth")

	// Prepare OAuth request
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	data.Set("audience", "https://api.devcycle.com/")

	req, err := http.NewRequestWithContext(ctx, "POST", authURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to parse auth response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	// Set expiry with a buffer to avoid edge cases
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	logger.Default.Debug("DevCycle: Authentication successful")
	return nil
}

// doRequest performs an authenticated API request
func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	// Ensure we have a valid token
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, apiURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.tokenMu.RLock()
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	c.tokenMu.RUnlock()

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// GetVariables fetches all variables for a project
func (c *Client) GetVariables(ctx context.Context, project string) ([]Variable, error) {
	logger.Default.Debug(fmt.Sprintf("DevCycle: Fetching variables for project %s", project))

	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/v1/projects/%s/variables", project), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch variables (status %d): %s", resp.StatusCode, string(body))
	}

	var variables []Variable
	if err := json.NewDecoder(resp.Body).Decode(&variables); err != nil {
		return nil, fmt.Errorf("failed to parse variables response: %w", err)
	}

	logger.Default.Debug(fmt.Sprintf("DevCycle: Fetched %d variables", len(variables)))
	return variables, nil
}

// GetFeatures fetches all features for a project
func (c *Client) GetFeatures(ctx context.Context, project string) ([]Feature, error) {
	logger.Default.Debug(fmt.Sprintf("DevCycle: Fetching features for project %s", project))

	resp, err := c.doRequest(ctx, "GET", fmt.Sprintf("/v2/projects/%s/features", project), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch features (status %d): %s", resp.StatusCode, string(body))
	}

	var features []Feature
	if err := json.NewDecoder(resp.Body).Decode(&features); err != nil {
		return nil, fmt.Errorf("failed to parse features response: %w", err)
	}

	logger.Default.Debug(fmt.Sprintf("DevCycle: Fetched %d features", len(features)))
	return features, nil
}

// CreateFeatureWithVariable creates a new feature with a variable in DevCycle
func (c *Client) CreateFeatureWithVariable(ctx context.Context, project string, variable Variable) error {
	logger.Default.Debug(fmt.Sprintf("DevCycle: Creating feature with variable %s", variable.Key))

	// DevCycle requires creating a feature first, which includes the variable
	feature := Feature{
		Key:         variable.Key,
		Name:        variable.Key,
		Description: variable.Description,
		Type:        "release",
		Variables: []Variable{
			{
				Key:          variable.Key,
				Name:         variable.Key,
				Type:         variable.Type,
				Description:  variable.Description,
				DefaultValue: variable.DefaultValue,
			},
		},
		Variations: []any{
			map[string]any{
				"key":  "variation-on",
				"name": "Variation On",
				"variables": map[string]any{
					variable.Key: getOnValue(variable.Type, variable.DefaultValue),
				},
			},
			map[string]any{
				"key":  "variation-off",
				"name": "Variation Off",
				"variables": map[string]any{
					variable.Key: variable.DefaultValue,
				},
			},
		},
	}

	resp, err := c.doRequest(ctx, "POST", fmt.Sprintf("/v2/projects/%s/features", project), feature)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create feature (status %d): %s", resp.StatusCode, string(body))
	}

	logger.Default.Debug(fmt.Sprintf("DevCycle: Successfully created feature %s", variable.Key))
	return nil
}

// UpdateVariable updates an existing variable in DevCycle
func (c *Client) UpdateVariable(ctx context.Context, project, variableKey string, variable Variable) error {
	logger.Default.Debug(fmt.Sprintf("DevCycle: Updating variable %s", variableKey))

	updateBody := map[string]any{
		"description": variable.Description,
	}

	resp, err := c.doRequest(ctx, "PATCH", fmt.Sprintf("/v1/projects/%s/variables/%s", project, variableKey), updateBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update variable (status %d): %s", resp.StatusCode, string(body))
	}

	logger.Default.Debug(fmt.Sprintf("DevCycle: Successfully updated variable %s", variableKey))
	return nil
}

// getOnValue returns the "on" value for a given type
func getOnValue(varType string, defaultValue any) any {
	switch varType {
	case "Boolean":
		// Return opposite of default
		if b, ok := defaultValue.(bool); ok {
			return !b
		}
		return true
	case "String":
		return "enabled"
	case "Number":
		return 1
	case "JSON":
		return map[string]any{"enabled": true}
	default:
		return defaultValue
	}
}
