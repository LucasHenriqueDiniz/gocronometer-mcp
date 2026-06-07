package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jrmycanady/gocronometer"
)

const protocolVersion = "2024-11-05"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type server struct {
	username         string
	password         string
	location         *time.Location
	cachedClient     *gocronometer.Client
	lastLoginAttempt time.Time
	lastLoginErr     error
}

type dateRangeArgs struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	RawCSV    bool   `json:"raw_csv"`
}

type dayArgs struct {
	Date string `json:"date"`
}

type dayInfoArgs struct {
	Date   string `json:"date"`
	RawGWT bool   `json:"raw_gwt"`
}

type searchFoodsArgs struct {
	Query       string `json:"query"`
	MaxResults  int    `json:"max_results"`
	SelectedTab string `json:"selected_tab"`
	CategoryID  int    `json:"category_id"`
	Type        string `json:"type"`
}

type removeServingArgs struct {
	ServingID string `json:"serving_id"`
	Confirm   bool   `json:"confirm"`
}

type addWeightArgs struct {
	Date    string  `json:"date"`
	Weight  float64 `json:"weight"`
	Unit    string  `json:"unit"`
	Confirm bool    `json:"confirm"`
	DryRun  *bool   `json:"dry_run"`
}

type canIEatArgs struct {
	Date          string   `json:"date"`
	Food          string   `json:"food"`
	EnergyKcal    *float64 `json:"energy_kcal"`
	ProteinG      *float64 `json:"protein_g"`
	CarbsG        *float64 `json:"carbs_g"`
	FatG          *float64 `json:"fat_g"`
	TargetKcal    *float64 `json:"target_kcal"`
	TargetProtein *float64 `json:"target_protein_g"`
	TargetCarbs   *float64 `json:"target_carbs_g"`
	TargetFat     *float64 `json:"target_fat_g"`
}

type addFoodArgs struct {
	Date      string   `json:"date"`
	Food      string   `json:"food"`
	FoodID    int      `json:"food_id"`
	MeasureID int      `json:"measure_id"`
	Amount    float64  `json:"amount"`
	Unit      string   `json:"unit"`
	Group     string   `json:"group"`
	Time      string   `json:"time"`
	Notes     string   `json:"notes"`
	Confirm   bool     `json:"confirm"`
	DryRun    *bool    `json:"dry_run"`
	Calories  *float64 `json:"calories"`
	ProteinG  *float64 `json:"protein_g"`
	CarbsG    *float64 `json:"carbs_g"`
	FatG      *float64 `json:"fat_g"`
}

type macroSummary struct {
	Date       string  `json:"date"`
	EnergyKcal float64 `json:"energy_kcal"`
	ProteinG   float64 `json:"protein_g"`
	CarbsG     float64 `json:"carbs_g"`
	NetCarbsG  float64 `json:"net_carbs_g"`
	FatG       float64 `json:"fat_g"`
	FiberG     float64 `json:"fiber_g"`
	SugarsG    float64 `json:"sugars_g"`
	SodiumMg   float64 `json:"sodium_mg"`
	FoodCount  int     `json:"food_count"`
}

func main() {
	if err := loadDotEnv(envDefault("CRONOMETER_ENV_FILE", ".env")); err != nil {
		fmt.Fprintf(os.Stderr, "gocronometer-mcp: %s\n", err)
		os.Exit(1)
	}

	locName := envDefault("CRONOMETER_TIMEZONE", "Local")
	loc, err := time.LoadLocation(locName)
	if err != nil || locName == "Local" {
		loc = time.Local
	}

	s := &server{
		username: os.Getenv("CRONOMETER_USERNAME"),
		password: os.Getenv("CRONOMETER_PASSWORD"),
		location: loc,
	}

	if err := s.serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "gocronometer-mcp: %s\n", err)
		os.Exit(1)
	}
}

func (s *server) serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(encoder, nil, nil, &rpcError{Code: -32700, Message: "parse error"})
			continue
		}

		if len(req.ID) == 0 {
			if req.Method == "notifications/initialized" {
				continue
			}
			continue
		}

		result, rpcErr := s.handle(req)
		writeResponse(encoder, req.ID, result, rpcErr)
	}

	return scanner.Err()
}

func writeResponse(encoder *json.Encoder, id json.RawMessage, result any, err *rpcError) {
	if id == nil {
		id = json.RawMessage("null")
	}
	_ = encoder.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	})
}

func (s *server) handle(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "gocronometer-mcp",
				"version": "0.1.0",
			},
		}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		var params callParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid tools/call params"}
		}
		return s.callTool(params)
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found"}
	}
}

func (s *server) callTool(params callParams) (any, *rpcError) {
	var (
		payload any
		err     error
	)

	switch params.Name {
	case "ping":
		payload = map[string]any{
			"status":             "ok",
			"server":             "gocronometer-mcp",
			"has_credentials":    s.username != "" && s.password != "",
			"write_command_set":  os.Getenv("CRONOMETER_ADD_FOOD_COMMAND") != "",
			"write_enabled_env":  os.Getenv("CRONOMETER_ENABLE_WRITE") == "true",
			"timezone":           s.location.String(),
			"supported_actions":  []string{"export", "macro_summary", "food_log_analysis", "native_add_food", "remove_serving", "add_weight"},
			"unsupported_native": []string{"diary_note_write_not_yet_captured"},
		}
	case "export_servings":
		var args dateRangeArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.exportServings(args)
		}
	case "macro_summary":
		var args dayArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.macroSummary(args)
		}
	case "analyze_day":
		var args dayArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.analyzeDay(args)
		}
	case "get_day_info":
		var args dayInfoArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.getDayInfo(args)
		}
	case "search_foods":
		var args searchFoodsArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.searchFoods(args)
		}
	case "can_i_eat":
		var args canIEatArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.canIEat(args)
		}
	case "add_food_to_diary":
		var args addFoodArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.addFood(args)
		}
	case "remove_serving":
		var args removeServingArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.removeServing(args)
		}
	case "add_weight":
		var args addWeightArgs
		err = decodeArgs(params.Arguments, &args)
		if err == nil {
			payload, err = s.addWeight(args)
		}
	default:
		return nil, &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}
	}

	if err != nil {
		return toolResult(map[string]any{"error": err.Error()}, true), nil
	}
	return toolResult(payload, false), nil
}

func toolResult(payload any, isError bool) any {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		raw = []byte(strconv.Quote(fmt.Sprint(payload)))
	}
	return map[string]any{
		"content": []textContent{{Type: "text", Text: string(raw)}},
		"isError": isError,
	}
}

func decodeArgs(raw json.RawMessage, target any) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	return json.Unmarshal(raw, target)
}

func (s *server) client(ctx context.Context) (*gocronometer.Client, error) {
	if s.username == "" || s.password == "" {
		return nil, errors.New("set CRONOMETER_USERNAME and CRONOMETER_PASSWORD before calling Cronometer tools")
	}
	if s.cachedClient != nil && s.cachedClient.UserID != "" && s.cachedClient.Nonce != "" {
		return s.cachedClient, nil
	}
	if s.lastLoginErr != nil && time.Since(s.lastLoginAttempt) < time.Minute {
		return nil, s.lastLoginErr
	}
	s.lastLoginAttempt = time.Now()
	c := gocronometer.NewClient(nil)
	if err := c.Login(ctx, s.username, s.password); err != nil {
		s.lastLoginErr = err
		return nil, err
	}
	s.lastLoginErr = nil
	s.cachedClient = c
	return c, nil
}

func (s *server) exportServings(args dateRangeArgs) (any, error) {
	start, end, err := parseRange(args.StartDate, args.EndDate, s.location)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := c.ExportServings(ctx, start, end)
	if err != nil {
		return nil, err
	}
	if args.RawCSV {
		return map[string]any{"start_date": args.StartDate, "end_date": args.EndDate, "csv": raw}, nil
	}

	records, err := gocronometer.ParseServingsExport(strings.NewReader(raw), s.location)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"start_date": args.StartDate,
		"end_date":   args.EndDate,
		"records":    records,
	}, nil
}

func (s *server) macroSummary(args dayArgs) (any, error) {
	args.Date = normalizeDate(args.Date, s.location)
	records, err := s.recordsForDay(args.Date)
	if err != nil {
		return nil, err
	}
	summary := summarize(args.Date, records)
	return summary, nil
}

func (s *server) analyzeDay(args dayArgs) (any, error) {
	args.Date = normalizeDate(args.Date, s.location)
	records, err := s.recordsForDay(args.Date)
	if err != nil {
		return nil, err
	}

	byGroup := make(map[string]macroSummary)
	topEnergy := make([]map[string]any, 0, len(records))
	for _, r := range records {
		group := r.Group
		if group == "" {
			group = "Ungrouped"
		}
		groupSummary := byGroup[group]
		groupSummary.Date = args.Date
		groupSummary.EnergyKcal += r.EnergyKcal
		groupSummary.ProteinG += r.ProteinG
		groupSummary.CarbsG += r.CarbsG
		groupSummary.NetCarbsG += r.NetCarbsG
		groupSummary.FatG += r.FatG
		groupSummary.FiberG += r.FiberG
		groupSummary.SugarsG += r.SugarsG
		groupSummary.SodiumMg += r.SodiumMg
		groupSummary.FoodCount++
		byGroup[group] = groupSummary

		topEnergy = append(topEnergy, map[string]any{
			"food":        r.FoodName,
			"group":       group,
			"energy_kcal": r.EnergyKcal,
			"protein_g":   r.ProteinG,
			"carbs_g":     r.CarbsG,
			"fat_g":       r.FatG,
		})
	}

	sortFoods(topEnergy)
	if len(topEnergy) > 10 {
		topEnergy = topEnergy[:10]
	}

	return map[string]any{
		"summary":          summarize(args.Date, records),
		"groups":           byGroup,
		"top_energy_foods": topEnergy,
		"note":             "Use the model's nutrition judgment on top of these numbers; this tool only summarizes Cronometer exports.",
	}, nil
}

func (s *server) searchFoods(args searchFoodsArgs) (any, error) {
	if strings.TrimSpace(args.Query) == "" {
		return nil, errors.New("query is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	results, err := c.SearchFoodsWithOptions(ctx, gocronometer.FoodSearchOptions{
		Query:       args.Query,
		MaxResults:  args.MaxResults,
		SelectedTab: args.SelectedTab,
		CategoryID:  args.CategoryID,
		Type:        args.Type,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"query":   args.Query,
		"results": results,
		"note":    "Use food_id and measure_id from the selected result when adding food. For exact units not shown here, capture another HAR or pass a known measure_id.",
	}, nil
}

func (s *server) getDayInfo(args dayInfoArgs) (any, error) {
	args.Date = normalizeDate(args.Date, s.location)
	date, err := time.ParseInLocation("2006-01-02", args.Date, s.location)
	if err != nil {
		return nil, fmt.Errorf("invalid date: use YYYY-MM-DD")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	result, err := c.GetDayInfo(ctx, date)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"date":        args.Date,
		"serving_ids": result.ServingIDs,
		"note":        "Use these IDs with remove_serving. Raw GWT parsing is intentionally minimal.",
	}
	if args.RawGWT {
		payload["raw_gwt"] = result.Raw
	}
	return payload, nil
}

func (s *server) canIEat(args canIEatArgs) (any, error) {
	if strings.TrimSpace(args.Food) == "" {
		return nil, errors.New("food is required")
	}
	args.Date = normalizeDate(args.Date, s.location)
	records, err := s.recordsForDay(args.Date)
	if err != nil {
		return nil, err
	}
	current := summarize(args.Date, records)
	planned := macroSummary{
		Date:       args.Date,
		EnergyKcal: value(args.EnergyKcal),
		ProteinG:   value(args.ProteinG),
		CarbsG:     value(args.CarbsG),
		FatG:       value(args.FatG),
	}
	after := current
	after.EnergyKcal += planned.EnergyKcal
	after.ProteinG += planned.ProteinG
	after.CarbsG += planned.CarbsG
	after.FatG += planned.FatG

	targets := map[string]float64{
		"energy_kcal": value(args.TargetKcal),
		"protein_g":   value(args.TargetProtein),
		"carbs_g":     value(args.TargetCarbs),
		"fat_g":       value(args.TargetFat),
	}

	flags := make([]string, 0)
	addFlag := func(label string, target float64, afterValue float64) {
		if target > 0 && afterValue > target {
			flags = append(flags, fmt.Sprintf("%s would exceed target by %.1f", label, afterValue-target))
		}
	}
	addFlag("energy", targets["energy_kcal"], after.EnergyKcal)
	addFlag("carbs", targets["carbs_g"], after.CarbsG)
	addFlag("fat", targets["fat_g"], after.FatG)

	recommendation := "ok_if_it_fits_your_preferences"
	if len(flags) > 0 {
		recommendation = "caution"
	}
	if args.TargetProtein != nil && planned.ProteinG > 0 && after.ProteinG < *args.TargetProtein {
		flags = append(flags, "protein target would still not be met")
	}

	return map[string]any{
		"food":           args.Food,
		"recommendation": recommendation,
		"flags":          flags,
		"current":        current,
		"planned":        planned,
		"after":          after,
		"targets":        targets,
		"note":           "This is arithmetic support, not medical advice. The model should explain tradeoffs and ask for missing target data when needed.",
	}, nil
}

func (s *server) addFood(args addFoodArgs) (any, error) {
	if strings.TrimSpace(args.Food) == "" {
		return nil, errors.New("food is required")
	}
	if args.Amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}
	dryRun := true
	if args.DryRun != nil {
		dryRun = *args.DryRun
	}

	payload := map[string]any{
		"date":       args.Date,
		"food":       args.Food,
		"food_id":    args.FoodID,
		"measure_id": args.MeasureID,
		"amount":     args.Amount,
		"unit":       args.Unit,
		"group":      args.Group,
		"time":       args.Time,
		"notes":      args.Notes,
		"calories":   args.Calories,
		"protein_g":  args.ProteinG,
		"carbs_g":    args.CarbsG,
		"fat_g":      args.FatG,
	}

	if dryRun {
		return map[string]any{
			"status":  "dry_run",
			"payload": payload,
			"next":    "Call again with dry_run=false, confirm=true, and CRONOMETER_ENABLE_WRITE=true. If food_id or measure_id are omitted, the MCP searches Cronometer and uses the first result's default measure.",
		}, nil
	}
	if !args.Confirm {
		return nil, errors.New("confirm=true is required for write attempts")
	}
	if os.Getenv("CRONOMETER_ENABLE_WRITE") != "true" {
		return nil, errors.New("CRONOMETER_ENABLE_WRITE=true is required for write attempts")
	}
	command := os.Getenv("CRONOMETER_ADD_FOOD_COMMAND")
	if command == "" {
		return s.addFoodNative(args)
	}

	raw, _ := json.Marshal(payload)
	cmd := exec.Command(command)
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("write command failed: %s: %s", err, strings.TrimSpace(string(output)))
	}
	return map[string]any{
		"status":  "executed",
		"payload": payload,
		"output":  strings.TrimSpace(string(output)),
	}, nil
}

func (s *server) addFoodNative(args addFoodArgs) (any, error) {
	args.Date = normalizeDate(args.Date, s.location)
	date, err := time.ParseInLocation("2006-01-02", args.Date, s.location)
	if err != nil {
		return nil, fmt.Errorf("invalid date: use YYYY-MM-DD")
	}
	hour, minute, err := parseClock(args.Time)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	selected := map[string]any{}
	foodID := args.FoodID
	measureID := args.MeasureID
	if foodID <= 0 || measureID <= 0 {
		searchOpts := gocronometer.FoodSearchOptions{Query: args.Food, MaxResults: 1}
		if isWaterQuery(args.Food) {
			searchOpts.SelectedTab = "BEVERAGES"
			searchOpts.CategoryID = 4
		}
		results, err := c.SearchFoodsWithOptions(ctx, searchOpts)
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no Cronometer food search result found for %q", args.Food)
		}
		foodID = results[0].ID
		if measureID <= 0 {
			measureID = results[0].MeasureID
		}
		selected = map[string]any{
			"name":                 results[0].Name,
			"food_id":              results[0].ID,
			"measure_id":           results[0].MeasureID,
			"measure_display_name": results[0].MeasureDisplayName,
			"source":               results[0].Source,
		}
	}

	result, err := c.AddServing(ctx, gocronometer.AddServingOptions{
		Date:      date,
		GroupID:   groupID(args.Group),
		FoodID:    foodID,
		MeasureID: measureID,
		Amount:    args.Amount,
		Hour:      hour,
		Minute:    minute,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status":     "added",
		"date":       args.Date,
		"group":      args.Group,
		"group_id":   groupID(args.Group),
		"food_id":    foodID,
		"measure_id": measureID,
		"amount":     args.Amount,
		"time":       args.Time,
		"selected":   selected,
		"serving_id": result.ServingID,
	}, nil
}

func (s *server) removeServing(args removeServingArgs) (any, error) {
	if strings.TrimSpace(args.ServingID) == "" {
		return nil, errors.New("serving_id is required")
	}
	if !args.Confirm {
		return nil, errors.New("confirm=true is required for remove_serving")
	}
	if os.Getenv("CRONOMETER_ENABLE_WRITE") != "true" {
		return nil, errors.New("CRONOMETER_ENABLE_WRITE=true is required for remove_serving")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.RemoveServing(ctx, args.ServingID); err != nil {
		return nil, err
	}
	return map[string]any{
		"status":     "removed",
		"serving_id": args.ServingID,
	}, nil
}

func (s *server) addWeight(args addWeightArgs) (any, error) {
	if args.Weight <= 0 {
		return nil, errors.New("weight must be greater than zero")
	}
	unit := strings.ToLower(strings.TrimSpace(args.Unit))
	if unit == "" {
		unit = "kg"
	}
	if unit != "kg" {
		return nil, errors.New("only kg is currently verified by the captured HAR")
	}
	args.Date = normalizeDate(args.Date, s.location)
	dryRun := true
	if args.DryRun != nil {
		dryRun = *args.DryRun
	}
	payload := map[string]any{
		"date":   args.Date,
		"weight": args.Weight,
		"unit":   unit,
	}
	if dryRun {
		return map[string]any{
			"status":  "dry_run",
			"payload": payload,
			"next":    "Call again with dry_run=false, confirm=true, and CRONOMETER_ENABLE_WRITE=true to add this weight biometric.",
		}, nil
	}
	if !args.Confirm {
		return nil, errors.New("confirm=true is required for add_weight")
	}
	if os.Getenv("CRONOMETER_ENABLE_WRITE") != "true" {
		return nil, errors.New("CRONOMETER_ENABLE_WRITE=true is required for add_weight")
	}
	date, err := time.ParseInLocation("2006-01-02", args.Date, s.location)
	if err != nil {
		return nil, fmt.Errorf("invalid date: use YYYY-MM-DD")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	result, err := c.AddWeightBiometric(ctx, date, args.Weight)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status":       "added",
		"date":         args.Date,
		"weight":       args.Weight,
		"unit":         unit,
		"biometric_id": result.BiometricID,
	}, nil
}

func (s *server) recordsForDay(date string) (gocronometer.ServingRecords, error) {
	start, end, err := parseRange(date, date, s.location)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return c.ExportServingsParsedWithLocation(ctx, start, end, s.location)
}

func summarize(date string, records gocronometer.ServingRecords) macroSummary {
	out := macroSummary{Date: date, FoodCount: len(records)}
	for _, r := range records {
		out.EnergyKcal += r.EnergyKcal
		out.ProteinG += r.ProteinG
		out.CarbsG += r.CarbsG
		out.NetCarbsG += r.NetCarbsG
		out.FatG += r.FatG
		out.FiberG += r.FiberG
		out.SugarsG += r.SugarsG
		out.SodiumMg += r.SodiumMg
	}
	return out
}

func parseRange(startDate, endDate string, loc *time.Location) (time.Time, time.Time, error) {
	if startDate == "" {
		startDate = time.Now().In(loc).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = startDate
	}
	start, err := time.ParseInLocation("2006-01-02", startDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_date: use YYYY-MM-DD")
	}
	end, err := time.ParseInLocation("2006-01-02", endDate, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end_date: use YYYY-MM-DD")
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end_date must be on or after start_date")
	}
	return start, end, nil
}

func tools() []tool {
	return []tool{
		{
			Name:        "ping",
			Description: "Check that the Cronometer MCP server is running and report credential/write configuration.",
			InputSchema: objectSchema(nil, nil),
		},
		{
			Name:        "export_servings",
			Description: "Export Cronometer serving records for a date range. Requires CRONOMETER_USERNAME and CRONOMETER_PASSWORD.",
			InputSchema: objectSchema(map[string]any{
				"start_date": stringProp("Start date in YYYY-MM-DD. Defaults to today."),
				"end_date":   stringProp("End date in YYYY-MM-DD. Defaults to start_date."),
				"raw_csv":    boolProp("Return raw CSV instead of parsed records."),
			}, nil),
		},
		{
			Name:        "macro_summary",
			Description: "Summarize calories, protein, carbs, fat, fiber, sugar, sodium, and food count for one day.",
			InputSchema: objectSchema(map[string]any{
				"date": stringProp("Date in YYYY-MM-DD. Defaults to today."),
			}, nil),
		},
		{
			Name:        "analyze_day",
			Description: "Return day macro totals, group totals, and top foods by energy for nutrition analysis.",
			InputSchema: objectSchema(map[string]any{
				"date": stringProp("Date in YYYY-MM-DD. Defaults to today."),
			}, nil),
		},
		{
			Name:        "search_foods",
			Description: "Search Cronometer foods and return IDs/measures that can be used by add_food_to_diary.",
			InputSchema: objectSchema(map[string]any{
				"query":        stringProp("Food search query, e.g. banana."),
				"max_results":  numberProp("Maximum results. Defaults to 10."),
				"selected_tab": stringProp("Optional Cronometer tab, e.g. ALL, FAVOURITES, COMMON_FOODS, BEVERAGES, SUPPLEMENTS, PRODUCTS, RESTAURANT, CUSTOM."),
				"category_id":  numberProp("Optional Cronometer category ID. BEVERAGES used 4 in the captured HAR."),
				"type":         stringProp("Optional Cronometer food type. Defaults to All."),
			}, []string{"query"}),
		},
		{
			Name:        "get_day_info",
			Description: "Load Cronometer diary day info and return serving IDs that can be passed to remove_serving.",
			InputSchema: objectSchema(map[string]any{
				"date":    stringProp("Date in YYYY-MM-DD. Defaults to today."),
				"raw_gwt": boolProp("Include raw GWT response for debugging."),
			}, nil),
		},
		{
			Name:        "can_i_eat",
			Description: "Compare a planned food against current day macros and optional targets. This performs arithmetic support only.",
			InputSchema: objectSchema(map[string]any{
				"date":             stringProp("Date in YYYY-MM-DD. Defaults to today."),
				"food":             stringProp("Food name to evaluate."),
				"energy_kcal":      numberProp("Planned calories."),
				"protein_g":        numberProp("Planned protein grams."),
				"carbs_g":          numberProp("Planned carb grams."),
				"fat_g":            numberProp("Planned fat grams."),
				"target_kcal":      numberProp("Optional daily calorie target."),
				"target_protein_g": numberProp("Optional daily protein target."),
				"target_carbs_g":   numberProp("Optional daily carb target."),
				"target_fat_g":     numberProp("Optional daily fat target."),
			}, []string{"food"}),
		},
		{
			Name:        "add_food_to_diary",
			Description: "Prepare or execute a Cronometer food diary write. Native write support needs a configured CRONOMETER_ADD_FOOD_COMMAND because this repository only exposes exports.",
			InputSchema: objectSchema(map[string]any{
				"date":       stringProp("Diary date in YYYY-MM-DD. Defaults are handled by the configured write command."),
				"food":       stringProp("Food name."),
				"food_id":    numberProp("Optional Cronometer food ID. If omitted, the MCP searches by food name."),
				"measure_id": numberProp("Optional Cronometer measure ID. If omitted, the first search result's default measure is used."),
				"amount":     numberProp("Amount to add. For Cronometer food measures captured so far, pass the base gram amount, even when the display unit is cup, tbsp, piece, or slice."),
				"unit":       stringProp("Serving unit, e.g. g, serving, cup."),
				"group":      stringProp("Diary group, e.g. Breakfast, Lunch, Dinner, Snacks."),
				"time":       stringProp("Optional time, e.g. 12:30."),
				"notes":      stringProp("Optional note."),
				"dry_run":    boolProp("Default true. Returns the payload without writing."),
				"confirm":    boolProp("Required true when dry_run=false."),
				"calories":   numberProp("Optional known calories for model/user confirmation."),
				"protein_g":  numberProp("Optional known protein grams."),
				"carbs_g":    numberProp("Optional known carb grams."),
				"fat_g":      numberProp("Optional known fat grams."),
			}, []string{"food", "amount"}),
		},
		{
			Name:        "remove_serving",
			Description: "Remove a Cronometer diary serving by serving_id returned from add_food_to_diary. Requires confirm=true and CRONOMETER_ENABLE_WRITE=true.",
			InputSchema: objectSchema(map[string]any{
				"serving_id": stringProp("Serving ID returned by add_food_to_diary."),
				"confirm":    boolProp("Required true to remove the serving."),
			}, []string{"serving_id", "confirm"}),
		},
		{
			Name:        "add_weight",
			Description: "Add a weight biometric. Requires confirm=true, dry_run=false, and CRONOMETER_ENABLE_WRITE=true. The captured HAR verifies kg.",
			InputSchema: objectSchema(map[string]any{
				"date":    stringProp("Date in YYYY-MM-DD. Defaults to today."),
				"weight":  numberProp("Weight amount."),
				"unit":    stringProp("Weight unit. Currently only kg is verified."),
				"dry_run": boolProp("Default true. Returns the payload without writing."),
				"confirm": boolProp("Required true when dry_run=false."),
			}, []string{"weight"}),
		},
	}
}

func groupID(group string) int {
	switch strings.ToLower(strings.TrimSpace(group)) {
	case "", "uncategorized", "sem categoria", "uncategorised":
		return 0
	case "breakfast", "cafe da manha", "café da manhã":
		return 1
	case "lunch", "almoco", "almoço":
		return 2
	case "dinner", "jantar":
		return 3
	case "snacks", "snack", "lanche", "lanches":
		return 4
	default:
		return 0
	}
}

func parseClock(value string) (*int, *int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil, nil
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid time: use HH:MM")
	}
	hour := parsed.Hour()
	minute := parsed.Minute()
	return &hour, &minute, nil
}

func isWaterQuery(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "water" || v == "agua" || v == "água"
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func numberProp(description string) map[string]any {
	return map[string]any{"type": "number", "description": description}
}

func boolProp(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func value(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func envDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func loadDotEnv(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("loading env file %s: %w", path, err)
	}
	content := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid env line %d in %s", i+1, path)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			return fmt.Errorf("empty env key on line %d in %s", i+1, path)
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	return nil
}

func normalizeDate(date string, loc *time.Location) string {
	if date != "" {
		return date
	}
	return time.Now().In(loc).Format("2006-01-02")
}

func sortFoods(items []map[string]any) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if asFloat(items[j]["energy_kcal"]) > asFloat(items[i]["energy_kcal"]) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	default:
		return 0
	}
}
