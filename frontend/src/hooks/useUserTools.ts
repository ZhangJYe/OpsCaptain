import { useCallback, useEffect, useState } from 'react'
import type { UserMCPTool, UserSkill } from '../types/userTools'
import { getApiBaseUrl } from '../lib/utils'

function unwrapPayload(data: any): any {
  if (data && typeof data === 'object' && data.data) {
    return data.data
  }
  return data
}

async function parseResponse<T>(res: Response): Promise<T> {
  const raw = await res.text()
  if (!raw.trim()) {
    return {} as T
  }
  const data = JSON.parse(raw)
  if (!res.ok) {
    throw new Error(String(data?.message || `HTTP ${res.status}`))
  }
  return unwrapPayload(data) as T
}

export function useUserTools() {
  const [tools, setTools] = useState<UserMCPTool[]>([])
  const [skills, setSkills] = useState<UserSkill[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const baseUrl = getApiBaseUrl()

  const refreshTools = useCallback(async () => {
    const res = await fetch(`${baseUrl}/mcp_tools`)
    const payload = await parseResponse<any>(res)
    setTools(Array.isArray(payload?.tools) ? payload.tools : Array.isArray(payload) ? payload : [])
  }, [baseUrl])

  const refreshSkills = useCallback(async () => {
    const res = await fetch(`${baseUrl}/skills`)
    const payload = await parseResponse<any>(res)
    setSkills(Array.isArray(payload?.skills) ? payload.skills : Array.isArray(payload) ? payload : [])
  }, [baseUrl])

  const refresh = useCallback(async () => {
    setIsLoading(true)
    setError(null)
    try {
      await Promise.all([refreshTools(), refreshSkills()])
    } catch (err: any) {
      setError(err?.message ?? String(err))
    } finally {
      setIsLoading(false)
    }
  }, [refreshTools, refreshSkills])

  useEffect(() => {
    refresh()
  }, [refresh])

  // --- Tool CRUD ---

  const createTool = useCallback(async (tool: Partial<UserMCPTool>) => {
    const res = await fetch(`${baseUrl}/mcp_tools`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(tool),
    })
    await parseResponse(res)
    await refreshTools()
  }, [baseUrl, refreshTools])

  const updateTool = useCallback(async (id: string, tool: Partial<UserMCPTool>) => {
    const res = await fetch(`${baseUrl}/mcp_tools/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(tool),
    })
    await parseResponse(res)
    await refreshTools()
  }, [baseUrl, refreshTools])

  const deleteTool = useCallback(async (id: string) => {
    const res = await fetch(`${baseUrl}/mcp_tools/${id}`, { method: 'DELETE' })
    await parseResponse(res)
    await refreshTools()
  }, [baseUrl, refreshTools])

  const testTool = useCallback(async (id: string) => {
    const res = await fetch(`${baseUrl}/mcp_tools/${id}/test`, { method: 'POST' })
    return parseResponse<any>(res)
  }, [baseUrl])

  const approveTool = useCallback(async (id: string) => {
    const res = await fetch(`${baseUrl}/mcp_tools/${id}/approve`, { method: 'POST' })
    await parseResponse(res)
    await refreshTools()
  }, [baseUrl, refreshTools])

  const rejectTool = useCallback(async (id: string) => {
    const res = await fetch(`${baseUrl}/mcp_tools/${id}/reject`, { method: 'POST' })
    await parseResponse(res)
    await refreshTools()
  }, [baseUrl, refreshTools])

  // --- Skill CRUD ---

  const createSkill = useCallback(async (skill: Partial<UserSkill>) => {
    const res = await fetch(`${baseUrl}/skills`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(skill),
    })
    await parseResponse(res)
    await refreshSkills()
  }, [baseUrl, refreshSkills])

  const updateSkill = useCallback(async (id: string, skill: Partial<UserSkill>) => {
    const res = await fetch(`${baseUrl}/skills/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(skill),
    })
    await parseResponse(res)
    await refreshSkills()
  }, [baseUrl, refreshSkills])

  const deleteSkill = useCallback(async (id: string) => {
    const res = await fetch(`${baseUrl}/skills/${id}`, { method: 'DELETE' })
    await parseResponse(res)
    await refreshSkills()
  }, [baseUrl, refreshSkills])

  const approveSkill = useCallback(async (id: string) => {
    const res = await fetch(`${baseUrl}/skills/${id}/approve`, { method: 'POST' })
    await parseResponse(res)
    await refreshSkills()
  }, [baseUrl, refreshSkills])

  const rejectSkill = useCallback(async (id: string) => {
    const res = await fetch(`${baseUrl}/skills/${id}/reject`, { method: 'POST' })
    await parseResponse(res)
    await refreshSkills()
  }, [baseUrl, refreshSkills])

  return {
    tools,
    skills,
    isLoading,
    error,
    refresh,
    createTool,
    updateTool,
    deleteTool,
    testTool,
    approveTool,
    rejectTool,
    createSkill,
    updateSkill,
    deleteSkill,
    approveSkill,
    rejectSkill,
  }
}
