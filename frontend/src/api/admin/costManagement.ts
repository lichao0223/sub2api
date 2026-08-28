import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'
import type { ChannelTimePricing } from './channels'

export interface CostPlan {
  id: number; name: string; plan_type: 'metered'|'fixed'; fixed_category?: string; status: string
  version_no: number; effective_from: string; effective_to?: string; billing_cycle: 'monthly'|'yearly'
  fixed_unit_cost_cny: string; monthly_unit_cost_cny: string
  subscription_unit_count: number
  model_count: number; account_count: number; note: string; prices?: CostModelPrice[]
}
export interface CostModelPrice {
  upstream_model: string; billing_mode: string; input_price_cny: string; output_price_cny: string
  cache_write_price_cny: string; cache_read_price_cny: string; image_input_price_cny: string
  image_output_price_cny: string; per_request_price_cny: string; time_pricing?: ChannelTimePricing | null
}
export interface AccountCostRow {
  account_id:number;account_name:string;platform:string;account_status:string;cost_mode:string
  plan_id?:number;plan_name:string;subscription_unit_id?:number;subscription_unit_name:string
  effective_from?:string;effective_to?:string;pending_count:number;exclude_reason:string
}
export interface CostSubscriptionUnit {
  id:number;plan_id:number;name:string;effective_from:string;effective_to?:string;billing_cycle:'monthly'|'yearly'
  fixed_unit_cost_cny:string;monthly_unit_cost_cny:string;version_no:number;price_effective_from:string;account_count:number
}
export interface CostPriceVersion {
  id:number;plan_id:number;subscription_unit_id?:number;subscription_unit_name:string;version_no:number
  effective_from:string;effective_to?:string;billing_cycle?:'monthly'|'yearly';fixed_unit_cost_cny:string
  monthly_unit_cost_cny:string;prices?:CostModelPrice[]
}
export interface CostOverview {
  dynamic_cost_cny:string;fixed_cost_cny:string;total_cost_cny:string;pending_count:number;error_count:number
  estimated_total_cost_cny?:string
  eligible_count:number;calculated_count:number;last_success_at?:string;coverage_start?:string;coverage_end?:string
  coverage_complete:boolean;previous_coverage_complete:boolean;previous_total_cost_cny:string
}
export interface CostBreakdownItem {
  cost_mode:'metered'|'fixed';plan_name:string;account_name:string;subscription_unit_name:string;upstream_model:string
  billing_mode:string;input_price_cny:string;output_price_cny:string;cache_write_price_cny:string;cache_read_price_cny:string;per_request_price_cny:string
  billing_cycle:string;fixed_unit_cost_cny:string;monthly_unit_cost_cny:string
  request_count:number;input_tokens:number;output_tokens:number;cache_write_tokens:number;cache_read_tokens:number;amount_cny:string
}
export interface CostBreakdown {
  total_cost_cny:string;items:CostBreakdownItem[]
}
export interface PendingCostDetail {
  account_id:number;account_name:string;start_date:string;end_date:string;issue_code:string;upstream_model:string;pending_count:number
}
export interface PendingCostDetails {
  total_count:number;items:PendingCostDetail[]
}
export interface CostAnalysis {
  period:string;total_cost_cny:string;trend:Array<{bucket:string;dynamic_cost_cny:string;fixed_cost_cny:string;total_cost_cny:string;plans:Array<{plan_id:number;plan_name:string;amount_cny:string}>}>
  top:Array<{plan_id:number;plan_name:string;amount_cny:string}>
}
export interface AccountCostInput {
  cost_mode:'metered'|'fixed'|'excluded';plan_id?:number;effective_from:string;effective_to?:string
  subscription_unit_id?:number;new_subscription_unit_name?:string;exclude_reason?:string;note?:string
}
export interface CostJob {
  id:number;kind:'incremental'|'recalculation';status:string;start_date?:string;end_date?:string;total_days:number;completed_days:number
  error_message:string;created_at:string;finished_at?:string
}

const base='/admin/cost-management'
export const costManagementAPI={
  overview:async(params:{start_date:string;end_date:string})=>(await apiClient.get<CostOverview>(`${base}/overview`,{params})).data,
  breakdown:async(params:{start_date:string;end_date:string;scope:'total'|'metered'|'fixed'})=>(await apiClient.get<CostBreakdown>(`${base}/breakdown`,{params})).data,
  pendingDetails:async(params:{start_date:string;end_date:string;account_id?:number})=>(await apiClient.get<PendingCostDetails>(`${base}/pending-details`,{params})).data,
  analysis:async(period:string)=>(await apiClient.get<CostAnalysis>(`${base}/analysis`,{params:{period}})).data,
  accounts:async(params:Record<string,unknown>)=>(await apiClient.get<BasePaginationResponse<AccountCostRow>>(`${base}/accounts`,{params})).data,
  modelOptions:async(params:Record<string,unknown>)=>(await apiClient.get<BasePaginationResponse<{model:string}>>(`${base}/model-options`,{params})).data,
  subscriptionUnits:async(plan_id:number)=>(await apiClient.get<CostSubscriptionUnit[]>(`${base}/subscription-units`,{params:{plan_id}})).data,
  createSubscriptionUnit:async(input:{plan_id:number;name:string;effective_from:string;billing_cycle:'monthly'|'yearly';fixed_unit_cost_cny:string})=>(await apiClient.post<CostSubscriptionUnit>(`${base}/subscription-units`,input)).data,
  renameSubscriptionUnit:async(id:number,name:string)=>(await apiClient.put(`${base}/subscription-units/${id}`,{name})).data,
  endSubscriptionUnit:async(id:number,effective_to:string)=>(await apiClient.post(`${base}/subscription-units/${id}/end`,{effective_to})).data,
  saveAccount:async(id:number,input:AccountCostInput)=>(await apiClient.put(`${base}/accounts/${id}`,input)).data,
  endAccount:async(id:number)=>(await apiClient.delete(`${base}/accounts/${id}`)).data,
  saveAccounts:async(account_ids:number[],input:AccountCostInput)=>(await apiClient.put(`${base}/accounts/batch`,{account_ids,...input})).data,
  plans:async(params:Record<string,unknown>)=>(await apiClient.get<BasePaginationResponse<CostPlan>>(`${base}/plans`,{params})).data,
  plan:async(id:number)=>(await apiClient.get<CostPlan>(`${base}/plans/${id}`)).data,
  createPlan:async(input:Record<string,unknown>)=>(await apiClient.post<CostPlan>(`${base}/plans`,input)).data,
  updatePlan:async(id:number,input:Record<string,unknown>)=>(await apiClient.put<CostPlan>(`${base}/plans/${id}`,input)).data,
  changePlanPrice:async(id:number,input:Record<string,unknown>)=>(await apiClient.post(`${base}/plans/${id}/price-changes`,input)).data,
  priceHistory:async(id:number)=>(await apiClient.get<CostPriceVersion[]>(`${base}/plans/${id}/price-history`)).data,
  disablePlan:async(id:number)=>(await apiClient.delete(`${base}/plans/${id}`)).data,
  userCosts:async(params:{start_date:string;end_date:string})=>(await apiClient.get<{items:Array<{user_id:number;dynamic_cost_cny:string;fixed_cost_cny:string;total_cost_cny:string}>;unallocated_fixed_cost_cny:string;platform_total_cost_cny:string}>(`${base}/user-costs`,{params})).data,
  recalculations:async(params:Record<string,unknown>)=>(await apiClient.get<BasePaginationResponse<CostJob>>(`${base}/recalculations`,{params})).data,
  createRecalculation:async(input:{start_date:string;end_date:string})=>(await apiClient.post(`${base}/recalculations`,input)).data,
  cancelRecalculation:async(id:number)=>(await apiClient.delete(`${base}/recalculations/${id}`)).data,
}
export default costManagementAPI
