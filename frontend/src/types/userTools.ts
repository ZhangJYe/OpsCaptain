export interface UserMCPTool {
  id: string
  name: string
  description: string
  transport: 'sse' | 'http'
  endpoint_url: string
  http_url?: string
  auth_token?: string
  tool_name: string
  input_schema?: Record<string, unknown>
  timeout_ms: number
  status: 'pending' | 'approved' | 'rejected' | 'disabled'
  created_at: string
  created_by: string
  approved_at?: string
  approved_by?: string
}

export interface UserSkill {
  id: string
  name: string
  description: string
  domain: 'metrics' | 'logs' | 'knowledge' | 'custom'
  tool_ref_id: string
  keywords: string[]
  focus?: string
  output_parser: 'json_array' | 'json_nested' | 'log_lines' | 'raw'
  json_path?: string
  tier: number
  status: 'pending' | 'approved' | 'rejected' | 'disabled'
  created_at: string
  created_by: string
  approved_at?: string
  approved_by?: string
}
