package gocronometer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const APIFoodSearchURL = "https://cronometer.com/api/v3/user/%s/food-search/string"

var addServingIDRegexp = regexp.MustCompile(`"([^"]+)"`)

type FoodSearchResult struct {
	Name               string `json:"name"`
	ID                 int    `json:"id"`
	Source             string `json:"source"`
	Type               string `json:"type"`
	Category           int    `json:"category"`
	MeasureID          int    `json:"measureId"`
	MeasureDisplayName string `json:"measureDisplayName"`
	DisplayString      string `json:"displayString"`
	ReplacementString  string `json:"replacementString"`
}

type AddServingOptions struct {
	Date      time.Time
	GroupID   int
	FoodID    int
	MeasureID int
	Amount    float64
	Hour      *int
	Minute    *int
}

type FoodSearchOptions struct {
	Query       string
	MaxResults  int
	Sources     string
	CategoryID  int
	SelectedTab string
	Type        string
}

type AddServingResult struct {
	ServingID string `json:"serving_id"`
	Raw       string `json:"raw"`
}

type DayInfoResult struct {
	ServingIDs []string `json:"serving_ids"`
	Raw        string   `json:"raw"`
}

type AddBiometricResult struct {
	BiometricID string `json:"biometric_id"`
	Raw         string `json:"raw"`
}

func (c *Client) SearchFoods(ctx context.Context, query string, maxResults int) ([]FoodSearchResult, error) {
	return c.SearchFoodsWithOptions(ctx, FoodSearchOptions{Query: query, MaxResults: maxResults})
}

func (c *Client) SearchFoodsWithOptions(ctx context.Context, opts FoodSearchOptions) ([]FoodSearchResult, error) {
	if c.UserID == "" {
		return nil, fmt.Errorf("client must be logged in before searching foods")
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}
	if opts.Sources == "" {
		opts.Sources = "All"
	}
	if opts.SelectedTab == "" {
		opts.SelectedTab = "ALL"
	}
	if opts.Type == "" {
		opts.Type = "All"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(APIFoodSearchURL, c.UserID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed while building food search request: %s", err)
	}
	q := url.Values{}
	q.Set("query", opts.Query)
	q.Set("maxResults", fmt.Sprintf("%d", opts.MaxResults))
	q.Set("sources", opts.Sources)
	q.Set("categoryId", fmt.Sprintf("%d", opts.CategoryID))
	q.Set("selectedTab", opts.SelectedTab)
	q.Set("type", opts.Type)
	req.URL.RawQuery = q.Encode()

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed while executing food search request: %s", err)
	}
	defer closeAndExhaustReader(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read food search response: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non 200 response of %d for food search: body [%s]", resp.StatusCode, string(body))
	}

	var results []FoodSearchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("failed to parse food search response: %s", err)
	}
	return results, nil
}

func (c *Client) AddServing(ctx context.Context, opts AddServingOptions) (AddServingResult, error) {
	if c.UserID == "" || c.Nonce == "" {
		return AddServingResult{}, fmt.Errorf("client must be logged in before adding servings")
	}
	if opts.FoodID <= 0 {
		return AddServingResult{}, fmt.Errorf("food ID is required")
	}
	if opts.MeasureID <= 0 {
		return AddServingResult{}, fmt.Errorf("measure ID is required")
	}
	amount := formatGWTFloat(opts.Amount)
	if opts.Date.IsZero() {
		opts.Date = time.Now()
	}

	header := c.GWTHeader
	if header == "" {
		header = GWTHeader
	}
	var reqBody string
	if opts.Hour != nil && opts.Minute != nil {
		reqBody = fmt.Sprintf(
			GWTUpdateDiaryAddServingWithTime,
			header,
			c.Nonce,
			c.UserID,
			opts.Date.Day(),
			int(opts.Date.Month()),
			opts.Date.Year(),
			opts.GroupID,
			*opts.Hour,
			*opts.Minute,
			c.UserID,
			amount,
			opts.FoodID,
			opts.MeasureID,
		)
	} else {
		reqBody = fmt.Sprintf(
			GWTUpdateDiaryAddServing,
			header,
			c.Nonce,
			c.UserID,
			opts.Date.Day(),
			int(opts.Date.Month()),
			opts.Date.Year(),
			opts.GroupID,
			amount,
			opts.FoodID,
			opts.MeasureID,
		)
	}

	req, err := c.NewGWTRequestWithContext(ctx, "POST", GWTBaseURL, strings.NewReader(reqBody))
	if err != nil {
		return AddServingResult{}, fmt.Errorf("failed while building update diary request: %s", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return AddServingResult{}, fmt.Errorf("failed while executing update diary request: %s", err)
	}
	defer closeAndExhaustReader(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AddServingResult{}, fmt.Errorf("failed to read update diary response: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return AddServingResult{}, fmt.Errorf("received non 200 response of %d for update diary: body [%s]", resp.StatusCode, string(body))
	}
	if strings.HasPrefix(string(body), "//EX") {
		return AddServingResult{}, fmt.Errorf("update diary returned exception: %s", string(body))
	}
	if !strings.HasPrefix(string(body), "//OK") {
		return AddServingResult{}, fmt.Errorf("update diary returned unexpected response: %s", string(body))
	}

	result := AddServingResult{Raw: string(body)}
	if match := addServingIDRegexp.FindStringSubmatch(string(body)); len(match) == 2 {
		result.ServingID = match[1]
	}
	return result, nil
}

func (c *Client) RemoveServing(ctx context.Context, servingID string) error {
	if c.UserID == "" || c.Nonce == "" {
		return fmt.Errorf("client must be logged in before removing servings")
	}
	if strings.TrimSpace(servingID) == "" {
		return fmt.Errorf("serving ID is required")
	}

	header := c.GWTHeader
	if header == "" {
		header = GWTHeader
	}
	reqBody := fmt.Sprintf(GWTRemoveServing, header, c.Nonce, servingID, c.UserID)
	req, err := c.NewGWTRequestWithContext(ctx, "POST", GWTBaseURL, strings.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed while building remove serving request: %s", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed while executing remove serving request: %s", err)
	}
	defer closeAndExhaustReader(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read remove serving response: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non 200 response of %d for remove serving: body [%s]", resp.StatusCode, string(body))
	}
	if strings.HasPrefix(string(body), "//EX") {
		return fmt.Errorf("remove serving returned exception: %s", string(body))
	}
	if !strings.HasPrefix(string(body), "//OK") {
		return fmt.Errorf("remove serving returned unexpected response: %s", string(body))
	}
	return nil
}

func (c *Client) GetDayInfo(ctx context.Context, date time.Time) (DayInfoResult, error) {
	if c.UserID == "" || c.Nonce == "" {
		return DayInfoResult{}, fmt.Errorf("client must be logged in before loading day info")
	}
	if date.IsZero() {
		date = time.Now()
	}

	header := c.GWTHeader
	if header == "" {
		header = GWTHeader
	}
	reqBody := fmt.Sprintf(GWTGetDayInfo, header, c.Nonce, date.Day(), int(date.Month()), date.Year(), c.UserID)
	req, err := c.NewGWTRequestWithContext(ctx, "POST", GWTBaseURL, strings.NewReader(reqBody))
	if err != nil {
		return DayInfoResult{}, fmt.Errorf("failed while building get day info request: %s", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return DayInfoResult{}, fmt.Errorf("failed while executing get day info request: %s", err)
	}
	defer closeAndExhaustReader(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DayInfoResult{}, fmt.Errorf("failed to read get day info response: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return DayInfoResult{}, fmt.Errorf("received non 200 response of %d for get day info: body [%s]", resp.StatusCode, string(body))
	}
	if strings.HasPrefix(string(body), "//EX") {
		return DayInfoResult{}, fmt.Errorf("get day info returned exception: %s", string(body))
	}
	if !strings.HasPrefix(string(body), "//OK") {
		return DayInfoResult{}, fmt.Errorf("get day info returned unexpected response: %s", string(body))
	}

	return DayInfoResult{
		ServingIDs: extractServingIDs(string(body)),
		Raw:        string(body),
	}, nil
}

func (c *Client) AddWeightBiometric(ctx context.Context, date time.Time, amount float64) (AddBiometricResult, error) {
	if c.UserID == "" || c.Nonce == "" {
		return AddBiometricResult{}, fmt.Errorf("client must be logged in before adding biometrics")
	}
	if amount <= 0 {
		return AddBiometricResult{}, fmt.Errorf("weight amount must be greater than zero")
	}
	if date.IsZero() {
		date = time.Now()
	}

	header := c.GWTHeader
	if header == "" {
		header = GWTHeader
	}
	reqBody := fmt.Sprintf(
		GWTAddWeightBiometric,
		header,
		c.Nonce,
		formatGWTFloat(amount),
		date.Day(),
		int(date.Month()),
		date.Year(),
		c.UserID,
	)
	req, err := c.NewGWTRequestWithContext(ctx, "POST", GWTBaseURL, strings.NewReader(reqBody))
	if err != nil {
		return AddBiometricResult{}, fmt.Errorf("failed while building add biometric request: %s", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return AddBiometricResult{}, fmt.Errorf("failed while executing add biometric request: %s", err)
	}
	defer closeAndExhaustReader(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AddBiometricResult{}, fmt.Errorf("failed to read add biometric response: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return AddBiometricResult{}, fmt.Errorf("received non 200 response of %d for add biometric: body [%s]", resp.StatusCode, string(body))
	}
	if strings.HasPrefix(string(body), "//EX") {
		return AddBiometricResult{}, fmt.Errorf("add biometric returned exception: %s", string(body))
	}
	if !strings.HasPrefix(string(body), "//OK") {
		return AddBiometricResult{}, fmt.Errorf("add biometric returned unexpected response: %s", string(body))
	}

	result := AddBiometricResult{Raw: string(body)}
	if match := addServingIDRegexp.FindStringSubmatch(string(body)); len(match) == 2 {
		result.BiometricID = match[1]
	}
	return result, nil
}

func extractServingIDs(raw string) []string {
	matches := addServingIDRegexp.FindAllStringSubmatch(raw, -1)
	ids := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		id := match[1]
		if strings.HasPrefix(id, "EW") && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

func formatGWTFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
