package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateCostPlanInputRejectsDuplicateModels(t *testing.T) {
	err := validateCostPlanInput(CostPlanInput{
		Name:          "Qwen 按量",
		PlanType:      "metered",
		EffectiveFrom: time.Now(),
		Prices: []CostModelPrice{
			{UpstreamModel: "qwen3"},
			{UpstreamModel: "qwen3"},
		},
	})
	require.EqualError(t, err, "duplicate upstream_model")
}

func TestValidateCostPlanInputRejectsInvalidPrice(t *testing.T) {
	err := validateCostPlanInput(CostPlanInput{
		Name:          "Qwen 按量",
		PlanType:      "metered",
		EffectiveFrom: time.Now(),
		Prices:        []CostModelPrice{{UpstreamModel: "qwen3", InputPriceCNY: "-1"}},
	})
	require.EqualError(t, err, "cost values must be nonnegative numbers")
}

func TestValidateCostPlanInputRejectsInvalidBillingCycle(t *testing.T) {
	err := validateCostPlanInput(CostPlanInput{
		Name:             "年度套餐",
		PlanType:         "fixed",
		FixedCategory:    "coding_plan",
		EffectiveFrom:    time.Now(),
		BillingCycle:     "weekly",
		FixedUnitCostCNY: "1200",
		PurchaseQuantity: 1,
	})
	require.EqualError(t, err, "invalid billing_cycle")
}

func TestCostPlanInputAcceptsNumericCostAmounts(t *testing.T) {
	var input CostPlanInput
	require.NoError(t, json.Unmarshal([]byte(`{
		"fixed_unit_cost_cny": 4500,
		"monthly_unit_cost_cny": "375",
		"prices": [{
			"upstream_model": "qwen",
			"input_price_cny": 1.25,
			"output_price_cny": "2.5",
			"per_request_price_cny": 0.1
		}]
	}`), &input))
	require.Equal(t, "4500", input.FixedUnitCostCNY)
	require.Equal(t, "375", input.MonthlyUnitCostCNY)
	require.Equal(t, "1.25", input.Prices[0].InputPriceCNY)
	require.Equal(t, "2.5", input.Prices[0].OutputPriceCNY)
	require.Equal(t, "0.1", input.Prices[0].PerRequestPriceCNY)
}

func TestCreateRecalculationRejectsToday(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	today := time.Now().In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)

	_, err = NewCostManagementService(nil, nil).CreateRecalculation(
		context.Background(), today.AddDate(0, 0, -1), today, 1,
	)
	require.EqualError(t, err, "历史补算的结束日期不能晚于昨天")
}
