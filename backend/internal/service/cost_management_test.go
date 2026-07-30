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
	})
	require.EqualError(t, err, "invalid billing_cycle")
}

func TestValidateFixedCostPlanDoesNotRequirePurchaseQuantity(t *testing.T) {
	require.NoError(t, validateCostPlanInput(CostPlanInput{
		Name: "ChatGPT Plus", PlanType: "fixed", FixedCategory: "coding_plan",
		EffectiveFrom: time.Now(), BillingCycle: "monthly", FixedUnitCostCNY: "140",
	}))
}

func TestValidateAccountCostInputRequiresOneFixedSubscriptionUnit(t *testing.T) {
	planID, unitID := int64(1), int64(2)
	base := AccountCostInput{
		AccountID: 1, CostMode: "fixed", PlanID: &planID, EffectiveFrom: time.Now(),
	}
	require.EqualError(t, validateAccountCostInput(base), "固定成本账号必须选择一个订阅实例")

	base.SubscriptionUnitID = &unitID
	require.NoError(t, validateAccountCostInput(base))

	base.NewSubscriptionUnitName = "订阅 #2"
	require.EqualError(t, validateAccountCostInput(base), "固定成本账号必须选择一个订阅实例")
}

func TestValidateSubscriptionUnitName(t *testing.T) {
	require.EqualError(t, validateSubscriptionUnitName(" \t "), "订阅实例名称不能为空")
	require.EqualError(t, validateSubscriptionUnitName(string(make([]rune, 121))), "订阅实例名称不能超过 120 个字符")
	require.NoError(t, validateSubscriptionUnitName("ChatGPT Plus #3"))
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

func TestCreateRecalculationRejectsFutureDate(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	today := time.Now().In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)

	_, err = NewCostManagementService(nil, nil).CreateRecalculation(
		context.Background(), today, today.AddDate(0, 0, 1), 1,
	)
	require.EqualError(t, err, "历史补算的结束日期不能晚于今天")
}
