package service

import (
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
