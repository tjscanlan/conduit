// Package weather is the one tool the tool-calling examples give their
// model. It returns canned data so the examples run offline and are
// deterministic.
package weather

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Tool metadata, shared by every provider's tool definition.
const (
	Name        = "get_weather"
	Description = "Get the current weather for a city."
	Question    = "What's the weather in Paris and in Tokyo right now, and which is warmer?"
)

// Schema is the tool's JSON Schema input: {"city": string}.
var Schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"city": map[string]any{"type": "string", "description": "City name, e.g. Paris"},
	},
	"required": []string{"city"},
}

var conditions = map[string]struct {
	TempC int    `json:"temp_c"`
	Sky   string `json:"sky"`
}{
	"paris":  {14, "light rain"},
	"tokyo":  {19, "clear"},
	"london": {11, "overcast"},
}

// Call runs the tool on a JSON input such as {"city":"Paris"} and returns
// its JSON result.
func Call(input json.RawMessage) (string, error) {
	var in struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.City == "" {
		return "", fmt.Errorf("get_weather: want {\"city\": string}, got %s", input)
	}
	c, ok := conditions[strings.ToLower(in.City)]
	if !ok {
		return "", fmt.Errorf("get_weather: no data for %q", in.City)
	}
	out, _ := json.Marshal(map[string]any{"city": in.City, "temp_c": c.TempC, "sky": c.Sky})
	return string(out), nil
}
