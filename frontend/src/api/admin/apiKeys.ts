/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient } from '../client'
import type { ApiKey, CreateApiKeyRequest, UpdateApiKeyRequest } from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
}

export interface BatchUpdateApiKeysRequest {
  group_id: number
  api_key_ids?: number[]
  all: boolean
  rate_limit_5h?: number
  rate_limit_1d?: number
  rate_limit_7d?: number
  concurrency_limit?: number
  status?: 'active' | 'inactive'
}

export async function batchUpdate(request: BatchUpdateApiKeysRequest): Promise<{ affected: number }> {
  const { data } = await apiClient.post<{ affected: number }>('/admin/api-keys/batch-update', request)
  return data
}

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key with auto-grant info
 */
export async function updateApiKeyGroup(id: number, groupId: number | null): Promise<UpdateApiKeyGroupResult> {
  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, {
    group_id: groupId === null ? 0 : groupId
  })
  return data
}

export async function createForUser(userId: number, payload: CreateApiKeyRequest): Promise<ApiKey> {
  const { data } = await apiClient.post<ApiKey>(`/admin/users/${userId}/api-keys`, payload)
  return data
}

export async function update(id: number, updates: UpdateApiKeyRequest): Promise<UpdateApiKeyGroupResult> {
  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, updates)
  return data
}

export async function deleteKey(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/api-keys/${id}`)
  return data
}

export async function rotateForUser(userId: number): Promise<ApiKey[]> {
  const { data } = await apiClient.post<{ api_keys: ApiKey[] }>(
    `/admin/users/${userId}/api-keys/rotate`
  )
  return data.api_keys
}

export const apiKeysAPI = {
  batchUpdate,
  updateApiKeyGroup,
  createForUser,
  update,
  rotateForUser,
  delete: deleteKey
}

export default apiKeysAPI
