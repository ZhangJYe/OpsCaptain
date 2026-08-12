import { useCallback, useEffect, useState } from 'react'
import { useTheme } from './hooks/useTheme'
import { useChat } from './hooks/useChat'
import { useIncidents } from './hooks/useIncidents'
import { MainLayout } from './components/layout/MainLayout'
import { AgentWorkbenchView } from './components/workbench/AgentWorkbenchView'
import { IncidentView } from './components/incident/IncidentView'
import { IncidentRecordList } from './components/incident/IncidentRecordList'
import { SettingsView } from './components/settings/SettingsView'
import { TopologyView } from './components/topology/TopologyView'
import { KnowledgeBaseView } from './components/knowledge/KnowledgeBaseView'
import { saveSession } from './lib/storage'
import { getApiBaseUrl } from './lib/utils'
import type { AIOpsEngine, AgentDiagnosisStrategy, AgentRouteDecision, AgentRouteMode, AgentRuntimeProfile, ChatSession, WorkbenchMode } from './types/chat'
import type { PetMood } from './components/pet/PetCharacter'

const SKILL_STORAGE_KEY = 'opscaptain-selected-skills'
const AIOPS_ENGINE_STORAGE_KEY = 'opscaptain-aiops-engine'
const AGENT_RUNTIME_PROFILE_STORAGE_KEY = 'opscaptain-agent-runtime-profile'
const WORKBENCH_MODE_STORAGE_KEY = 'opscaptain-workbench-mode'
const PET_ENABLED_KEY = 'opscaptain-pet-enabled'

function loadRuntimeProfile(): AgentRuntimeProfile {
  if (typeof window === 'undefined') return { routeMode: 'auto', diagnosisStrategy: 'auto' }
  try {
    const stored = JSON.parse(localStorage.getItem(AGENT_RUNTIME_PROFILE_STORAGE_KEY) || '{}')
    if (['auto', 'react', 'diagnosis'].includes(stored.routeMode)) {
      return {
        routeMode: stored.routeMode,
        diagnosisStrategy: ['auto', 'plan_execute_replan', 'gos_engine'].includes(stored.diagnosisStrategy)
          ? stored.diagnosisStrategy
          : 'plan_execute_replan',
      }
    }
  } catch {
    // Fall through to the former engine preference for a one-time migration.
  }
  return { routeMode: 'auto', diagnosisStrategy: 'auto' }
}

async function requestAgentRoute(query: string, routeMode: AgentRouteMode, diagnosisStrategy: AgentDiagnosisStrategy): Promise<AgentRouteDecision> {
  const res = await fetch(`${getApiBaseUrl()}/agent_route`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, route_mode: routeMode, diagnosis_strategy: diagnosisStrategy }),
  })
  const raw = await res.text()
  let payload: any = {}
  try {
    payload = JSON.parse(raw)
  } catch {
    throw new Error(raw || `HTTP ${res.status}`)
  }
  const data = payload?.data || payload
  if (!res.ok) throw new Error(String(payload?.message || data?.message || `HTTP ${res.status}`))
  if (!['chat', 'incident', 'confirm'].includes(data?.decision)) throw new Error('路由服务返回了未知结果')
  return data as AgentRouteDecision
}

export default function App() {
  const { theme, toggle: toggleTheme } = useTheme()
  const chat = useChat()
  const incidents = useIncidents()
  const [sidebarOpen, setSidebarOpen] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.innerWidth >= 1024
  })
  const [selectedSkillIds, setSelectedSkillIds] = useState<string[]>(() => {
    if (typeof window === 'undefined') return []
    try {
      const raw = localStorage.getItem(SKILL_STORAGE_KEY)
      const parsed = raw ? JSON.parse(raw) : []
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  })
  const [aiOpsEngine, setAIOpsEngine] = useState<AIOpsEngine>(() => {
    if (typeof window === 'undefined') return 'plan_execute_replan'
    const raw = localStorage.getItem(AIOPS_ENGINE_STORAGE_KEY)
    return raw === 'gos_engine' ? 'gos_engine' : 'plan_execute_replan'
  })
  const [runtimeProfile, setRuntimeProfile] = useState<AgentRuntimeProfile>(loadRuntimeProfile)
  const [isRouting, setIsRouting] = useState(false)
  const [pendingRoute, setPendingRoute] = useState<{ query: string; reason: string } | null>(null)
  const [incidentRecordView, setIncidentRecordView] = useState<'list' | 'detail'>('list')
  const [workbenchMode, setWorkbenchMode] = useState<WorkbenchMode>(() => {
    if (typeof window === 'undefined') return 'chat'
    const raw = localStorage.getItem(WORKBENCH_MODE_STORAGE_KEY)
    if (raw === 'chat' || raw === 'aiops' || raw === 'knowledge' || raw === 'topology') return raw
    return 'chat'
  })
  const [petEnabled, setPetEnabled] = useState<boolean>(() => {
    if (typeof window === 'undefined') return true
    const raw = localStorage.getItem(PET_ENABLED_KEY)
    return raw !== 'false'
  })
  const incidentEngine = incidents.incident?.engine_strategy === 'gos_engine' ? 'gos_engine' : 'plan_execute_replan'
  const displayedAIOpsEngine = incidents.incident && workbenchMode === 'aiops' && incidentRecordView === 'detail' ? incidentEngine : aiOpsEngine
  const agentWorking = workbenchMode === 'aiops' ? incidents.isLoading : chat.isLoading
  const petMood: PetMood = agentWorking
    ? (workbenchMode === 'aiops' ? displayedAIOpsEngine === 'gos_engine' : chat.loadingEngine === 'gos_engine')
      ? 'gos'
      : 'thinking'
    : 'idle'

  useEffect(() => {
    try {
      localStorage.setItem(SKILL_STORAGE_KEY, JSON.stringify(selectedSkillIds))
    } catch {
      return
    }
  }, [selectedSkillIds])

  useEffect(() => {
    try {
      localStorage.setItem(AIOPS_ENGINE_STORAGE_KEY, aiOpsEngine)
    } catch {
      return
    }
  }, [aiOpsEngine])

  useEffect(() => {
    try {
      localStorage.setItem(AGENT_RUNTIME_PROFILE_STORAGE_KEY, JSON.stringify(runtimeProfile))
    } catch {
      return
    }
  }, [runtimeProfile])

  useEffect(() => {
    try {
      localStorage.setItem(WORKBENCH_MODE_STORAGE_KEY, workbenchMode)
    } catch {
      return
    }
  }, [workbenchMode])

  useEffect(() => {
    try {
      localStorage.setItem(PET_ENABLED_KEY, String(petEnabled))
    } catch {
      return
    }
  }, [petEnabled])

  useEffect(() => {
    if (chat.messages.length === 0 || workbenchMode !== 'chat') {
      return
    }
    saveSession(chat.sessionId, chat.messages, {
      mode: chat.mode,
      workMode: 'react',
      selectedSkillIds,
    })
  }, [chat.sessionId, chat.messages, chat.mode, selectedSkillIds, workbenchMode])

  const startChat = useCallback((query: string) => {
    setWorkbenchMode('chat')
    chat.send(query, { selectedSkillIds })
  }, [chat, selectedSkillIds])

  const startIncident = useCallback((query: string, engine: AIOpsEngine) => {
    setWorkbenchMode('aiops')
    setIncidentRecordView('detail')
    void incidents.createIncident(query, engine).catch(() => undefined)
  }, [incidents])

  const decideAndStart = useCallback(async (query: string, routeMode: AgentRouteMode, diagnosisStrategy: AgentDiagnosisStrategy) => {
    const trimmed = query.trim()
    if (!trimmed || isRouting) return
    setIsRouting(true)
    try {
      const decision = await requestAgentRoute(trimmed, routeMode, diagnosisStrategy)
      if (decision.decision === 'chat') {
        startChat(trimmed)
      } else if (decision.decision === 'incident' && decision.strategy) {
        startIncident(trimmed, decision.strategy)
      } else {
        setPendingRoute({ query: trimmed, reason: decision.reason || '请确认这次请求要继续问答还是启动排障。' })
      }
    } catch {
      setPendingRoute({ query: trimmed, reason: '智能路由暂不可用，请手动选择本次请求的执行方式。' })
    } finally {
      setIsRouting(false)
    }
  }, [isRouting, startChat, startIncident])

  const handleSendChat = useCallback((query: string) => {
    if (runtimeProfile.routeMode === 'react') {
      startChat(query)
      return
    }
    if (runtimeProfile.routeMode === 'diagnosis') {
      void decideAndStart(query, 'diagnosis', runtimeProfile.diagnosisStrategy)
      return
    }
    void decideAndStart(query, 'auto', runtimeProfile.diagnosisStrategy)
  }, [decideAndStart, runtimeProfile, startChat])

  const handleAIOpsEngineChange = useCallback(
    (engine: AIOpsEngine) => {
      if (incidents.incident && workbenchMode === 'aiops' && incidentRecordView === 'detail') {
        return
      }
      setAIOpsEngine(engine)
      setRuntimeProfile((profile) => ({ ...profile, diagnosisStrategy: engine }))
      setWorkbenchMode('aiops')
    },
    [incidentRecordView, incidents.incident, workbenchMode],
  )

  const handleLoadSession = useCallback(
    (session: ChatSession) => {
      const loaded = chat.loadSession(session)
      if (!loaded) {
        return
      }
      setSelectedSkillIds(Array.isArray(session.selectedSkillIds) ? session.selectedSkillIds : [])
      setWorkbenchMode('chat')
    },
    [chat],
  )

  const handleLoadIncident = useCallback(
    (incidentId: string) => {
      setWorkbenchMode('aiops')
      setIncidentRecordView('detail')
      void incidents.loadIncident(incidentId).catch(() => undefined)
    },
    [incidents],
  )

  const handleNewChat = useCallback(() => {
    setWorkbenchMode('chat')
    chat.newSession()
  }, [chat])

  const handleWorkbenchModeChange = useCallback((mode: WorkbenchMode) => {
    setWorkbenchMode(mode)
    if (mode === 'aiops') {
      setIncidentRecordView('list')
      incidents.newIncident()
    }
  }, [incidents])

  const handleBackToIncidentList = useCallback(() => {
    incidents.newIncident()
    setIncidentRecordView('list')
  }, [incidents])

  return (
    <MainLayout
      theme={theme}
      sidebarOpen={sidebarOpen}
      onToggleSidebar={() => setSidebarOpen((value) => !value)}
      onCloseSidebar={() => setSidebarOpen(false)}
      onToggleTheme={toggleTheme}
      onNewChat={handleNewChat}
      onLoadSession={handleLoadSession}
      onLoadIncident={handleLoadIncident}
      chatMode={chat.mode}
      onModeChange={chat.setMode}
      workbenchMode={workbenchMode}
      onWorkbenchModeChange={handleWorkbenchModeChange}
      aiOpsEngine={displayedAIOpsEngine}
      runtimeProfile={runtimeProfile}
      onAIOpsEngineChange={handleAIOpsEngineChange}
      sessionId={chat.sessionId}
      currentIncidentId={incidents.incident?.incident_id || ''}
      currentIncidentEngine={incidents.incident?.engine_strategy || ''}
      incidents={incidents.incidents}
      messages={chat.messages}
      selectedSkillIds={selectedSkillIds}
      onSelectedSkillIdsChange={setSelectedSkillIds}
      isLoading={chat.isLoading || incidents.isLoading || isRouting}
      petEnabled={petEnabled}
      petMood={petMood}
    >
      {workbenchMode === 'settings' ? (
        <SettingsView
          onBack={() => setWorkbenchMode('chat')}
          runtimeProfile={runtimeProfile}
          onRuntimeProfileChange={setRuntimeProfile}
          activeIncidentEngine={incidents.incident?.engine_strategy || ''}
        />
      ) : workbenchMode === 'topology' ? (
        <TopologyView onBack={() => setWorkbenchMode('chat')} />
      ) : workbenchMode === 'knowledge' ? (
        <KnowledgeBaseView />
      ) : workbenchMode === 'aiops' ? (
        incidentRecordView === 'list' ? (
          <IncidentRecordList
            incidents={incidents.incidents}
            isLoading={incidents.isLoading || isRouting}
            onSelect={handleLoadIncident}
          />
        ) : (
          <IncidentView
            incident={incidents.incident}
            isLoading={incidents.isLoading || isRouting}
            error={incidents.error}
            engine={displayedAIOpsEngine}
            onBack={handleBackToIncidentList}
            onAppend={(query) => {
              void incidents.appendTurn(query).catch(() => undefined)
            }}
          />
        )
      ) : (
        <AgentWorkbenchView
          messages={chat.messages}
          streamingContent={chat.streamingContent}
          streamingThoughts={chat.streamingThoughts}
          thinkingSteps={chat.thinkingSteps}
          suggestions={chat.suggestions}
          isLoading={chat.isLoading || isRouting}
          loadingEngine={chat.loadingEngine}
          mode={chat.mode}
          workbenchMode="chat"
          runtimeProfile={runtimeProfile}
          selectedSkillIds={selectedSkillIds}
          petEnabled={petEnabled}
          aiOpsEngine={aiOpsEngine}
          onSend={handleSendChat}
          onStop={chat.stop}
          onModeChange={chat.setMode}
          onTogglePet={() => setPetEnabled((value) => !value)}
          onClearSuggestions={chat.clearSuggestions}
        />
      )}
      {pendingRoute && (
        <div className="fixed inset-0 z-[70] flex items-end justify-center bg-slate-950/20 p-4 backdrop-blur-[2px] sm:items-center">
          <div className="w-full max-w-md rounded-2xl border border-white/70 bg-white p-5 shadow-2xl shadow-slate-950/15 dark:border-slate-700 dark:bg-slate-900">
            <p className="text-xs font-semibold tracking-wide text-sky-600">需要你确认</p>
            <h2 className="mt-1 text-base font-semibold text-zinc-900 dark:text-white">这次请求应该怎么处理？</h2>
            <p className="mt-2 text-sm leading-6 text-zinc-500 dark:text-zinc-400">{pendingRoute.reason}</p>
            <div className="mt-5 grid grid-cols-2 gap-3">
              <button onClick={() => { startChat(pendingRoute.query); setPendingRoute(null) }} className="rounded-xl border border-sky-200 bg-sky-50 px-3 py-2.5 text-sm font-medium text-sky-700 transition hover:bg-sky-100 dark:border-sky-500/20 dark:bg-sky-500/10 dark:text-sky-300">继续问答</button>
              <button onClick={() => { startIncident(pendingRoute.query, aiOpsEngine); setPendingRoute(null) }} className="rounded-xl bg-slate-900 px-3 py-2.5 text-sm font-medium text-white transition hover:bg-slate-700 dark:bg-sky-500 dark:hover:bg-sky-400">启动排障</button>
            </div>
            <button onClick={() => setPendingRoute(null)} className="mt-3 w-full text-xs text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300">暂不处理</button>
          </div>
        </div>
      )}
    </MainLayout>
  )
}
