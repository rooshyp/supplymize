package outbound

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/supplymize/supplymize/services/control-plane/internal/app"
	"github.com/supplymize/supplymize/services/control-plane/internal/domain"
)

type ForecastClient struct {
	client *jsonClient
}

func NewForecastClient(baseURL string, httpClient HTTPDoer, logger *slog.Logger) (*ForecastClient, error) {
	client, err := newJSONClient("forecast", baseURL, httpClient, logger)
	if err != nil {
		return nil, err
	}
	return &ForecastClient{client: client}, nil
}

type forecastRequestDTO struct {
	SKU     string             `json:"sku"`
	History []float64          `json:"history"`
	Horizon int                `json:"horizon"`
	Signals forecastSignalsDTO `json:"signals"`
}

type impactSignalDTO struct {
	Impact float64 `json:"impact"`
}

type supplierSpeedSignalDTO struct {
	SpeedIndex float64 `json:"speed_index"`
}

type forecastSignalsDTO struct {
	Trend         *float64                 `json:"trend,omitempty"`
	Seasonality   []float64                `json:"seasonality,omitempty"`
	Weather       []impactSignalDTO        `json:"weather,omitempty"`
	Event         []impactSignalDTO        `json:"event,omitempty"`
	SupplierSpeed []supplierSpeedSignalDTO `json:"supplier_speed,omitempty"`
}

type forecastPointDTO struct {
	Period          string   `json:"period"`
	Step            int      `json:"step"`
	Date            string   `json:"date"`
	Demand          *float64 `json:"demand"`
	PredictedUnits  *float64 `json:"predicted_units"`
	PredictedDemand *float64 `json:"predicted_demand"`
	Units           *float64 `json:"units"`
	LowerBound      *float64 `json:"lower_bound"`
	Lower           *float64 `json:"lower"`
	UpperBound      *float64 `json:"upper_bound"`
	Upper           *float64 `json:"upper"`
}

type forecastPayloadDTO struct {
	Model       string             `json:"model"`
	TotalDemand *float64           `json:"total_demand"`
	Confidence  float64            `json:"confidence"`
	Points      []forecastPointDTO `json:"points"`
}

type forecastResponseDTO struct {
	SKU          string              `json:"sku"`
	Model        string              `json:"model"`
	TotalDemand  *float64            `json:"total_demand"`
	Confidence   json.RawMessage     `json:"confidence"`
	Points       []forecastPointDTO  `json:"points"`
	Forecast     *forecastPayloadDTO `json:"forecast"`
	GeneratedAt  string              `json:"generated_at"`
	ModelVersion string              `json:"model_version"`
}

func (c *ForecastClient) Forecast(ctx context.Context, input app.ForecastInput) (domain.Forecast, error) {
	history := make([]float64, 0, len(input.History))
	for _, observation := range input.History {
		history = append(history, observation.Units)
	}
	trend := input.Signals.Trend
	signals := forecastSignalsDTO{
		Trend:       &trend,
		Seasonality: append([]float64(nil), input.Signals.Seasonality...),
	}
	if input.Signals.Weather != 0 {
		signals.Weather = []impactSignalDTO{{Impact: input.Signals.Weather}}
	}
	if input.Signals.Event != 0 {
		signals.Event = []impactSignalDTO{{Impact: input.Signals.Event}}
	}
	if input.Signals.SupplierSpeed != 0 {
		signals.SupplierSpeed = []supplierSpeedSignalDTO{{SpeedIndex: input.Signals.SupplierSpeed}}
	}
	request := forecastRequestDTO{
		SKU:     input.SKU,
		History: history,
		Horizon: input.Horizon,
		Signals: signals,
	}
	var response forecastResponseDTO
	if err := c.client.do(ctx, http.MethodPost, "/v1/forecasts", request, &response); err != nil {
		return domain.Forecast{}, err
	}
	payload := forecastPayloadDTO{
		Model:       response.Model,
		TotalDemand: response.TotalDemand,
		Points:      response.Points,
	}
	rootConfidence, err := confidenceFromJSON(response.Confidence)
	if err != nil {
		return domain.Forecast{}, fmt.Errorf("decode forecast confidence: %w", err)
	}
	if response.Forecast != nil {
		payload = *response.Forecast
		if payload.Model == "" {
			payload.Model = response.Model
		}
	}
	if payload.Model == "" {
		payload.Model = response.ModelVersion
	}
	if len(response.Confidence) > 0 && string(response.Confidence) != "null" {
		payload.Confidence = rootConfidence
	}

	points := make([]domain.ForecastPoint, 0, len(payload.Points))
	calculatedTotal := 0.0
	for i, point := range payload.Points {
		demand, ok := firstFloat(point.Demand, point.PredictedUnits, point.PredictedDemand, point.Units)
		if !ok {
			return domain.Forecast{}, fmt.Errorf("forecast point %d did not include demand", i)
		}
		lower, ok := firstFloat(point.LowerBound, point.Lower)
		if !ok {
			lower = demand
		}
		upper, ok := firstFloat(point.UpperBound, point.Upper)
		if !ok {
			upper = demand
		}
		period := strings.TrimSpace(point.Period)
		if period == "" {
			period = strings.TrimSpace(point.Date)
		}
		if period == "" && point.Step > 0 {
			period = fmt.Sprintf("step-%d", point.Step)
		}
		points = append(points, domain.ForecastPoint{
			Period:     period,
			Demand:     demand,
			LowerBound: lower,
			UpperBound: upper,
		})
		calculatedTotal += demand
	}
	total := calculatedTotal
	if payload.TotalDemand != nil {
		total = *payload.TotalDemand
	}
	return domain.Forecast{
		Model:       strings.TrimSpace(payload.Model),
		TotalDemand: total,
		Confidence:  payload.Confidence,
		Points:      points,
	}, nil
}

func (c *ForecastClient) Ready(ctx context.Context) error {
	return c.client.do(ctx, http.MethodGet, "/readyz", nil, nil)
}

func firstFloat(values ...*float64) (float64, bool) {
	for _, value := range values {
		if value != nil {
			return *value, true
		}
	}
	return 0, false
}

func confidenceFromJSON(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var scalar float64
	if err := json.Unmarshal(raw, &scalar); err == nil {
		return scalar, nil
	}
	var summary struct {
		Score *float64 `json:"score"`
		Level *float64 `json:"level"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		return 0, err
	}
	if summary.Score != nil {
		return *summary.Score, nil
	}
	if summary.Level != nil {
		return *summary.Level, nil
	}
	return 0, fmt.Errorf("confidence object contains neither score nor level")
}
