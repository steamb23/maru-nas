import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchAddon, fetchJobOutput, fetchJobs, isPackagesResponse } from '../../api/client'
import type { Job, JobsResponse, PackagesResponse } from '../../api/types.gen'
import { reportActivity } from '../../auth/activity'
import { createPoller } from '../common/poller'
import { DEFAULT_TIMEOUT_MS, createSharedPoll } from '../common/sharedPoll'
import { type PolledData, usePolledData } from '../common/usePolledData'
import { type OutputBuffer, appendOutput, emptyOutput } from './packages'

// 업데이트 목록은 하루에 한 번쯤 바뀌므로(apt-daily, 확인·설치 작업) 창이 열린 동안 30초마다 본다.
export const PACKAGES_POLL_INTERVAL_MS = 30_000
// 작업 목록은 작업이 진행 중이면 자주, 아니면 드물게 본다. 출력은 진행 중인 작업을 1초마다 이어 받는다.
const JOBS_RUNNING_INTERVAL_MS = 3_000
const JOBS_IDLE_INTERVAL_MS = 30_000
const OUTPUT_INTERVAL_MS = 1_000
// 작업이 진행 중이고 창이 보이는 동안 세션 활동을 알리는 간격(세션 유지기가 60초에 한 번 연장한다).
// 감시자가 이 세션으로 출력을 읽으므로 지켜보는 동안 세션이 끝나지 않게 한다(OMV 워크벤치의 작업 창과 같다).
const ACTIVITY_MS = 30_000

const packagesPoll = createSharedPoll({
  fetch: (signal, refresh) => fetchAddon('packages', isPackagesResponse, { signal, refresh }),
  timeoutMs: DEFAULT_TIMEOUT_MS,
})

export function usePackages(active: boolean): PolledData<PackagesResponse> {
  return usePolledData(packagesPoll, active, PACKAGES_POLL_INTERVAL_MS)
}

export interface JobsData {
  jobs: JobsResponse | null
  // 마지막 요청이 실패했는지(받은 목록은 그대로 둔다).
  failed: boolean
  refresh: () => void
}

// useJobs는 진행 중인 작업과 이 사용자의 결과를 받는다. 창이 보이지 않으면 멈춘다.
export function useJobs(active: boolean): JobsData {
  const [jobs, setJobs] = useState<JobsResponse | null>(null)
  const [failed, setFailed] = useState(false)
  const running = useRef(false)
  const pollerRef = useRef<ReturnType<typeof createPoller> | null>(null)

  useEffect(() => {
    if (!active) return
    const poller = createPoller({
      fetch: fetchJobs,
      intervalMs: () => (running.current ? JOBS_RUNNING_INTERVAL_MS : JOBS_IDLE_INTERVAL_MS),
      timeoutMs: DEFAULT_TIMEOUT_MS,
      onData: (data) => {
        const was = running.current
        running.current = data.current !== null
        setJobs(data)
        setFailed(false)
        if (was !== running.current) poller.reschedule()
      },
      onError: () => setFailed(true),
    })
    pollerRef.current = poller
    poller.start()
    return () => {
      poller.stop()
      pollerRef.current = null
    }
  }, [active])

  const refresh = useCallback(() => {
    fetchJobs().then(
      (data) => {
        running.current = data.current !== null
        setJobs(data)
        setFailed(false)
        pollerRef.current?.reschedule()
      },
      () => setFailed(true),
    )
  }, [])

  return { jobs, failed, refresh }
}

export interface OutputData {
  output: OutputBuffer | null
  // waiting이면 서버가 관리자 세션이 없어 출력을 기다리는 중이다.
  waiting: boolean
}

// useJobOutput은 job의 출력을 커서로 이어 받는다. 진행 중이면 1초마다, 이미 받은 출력이 더 있으면 바로 다시 받고,
// 끝난 작업은 끝까지 받은 뒤 멈춘다. 창이 보이지 않으면 멈추며 커서라 다시 보이면 이어 받는다.
// 진행 중이던 작업이 끝나면 onEnded를 부른다(목록과 업데이트를 새로 받는다).
export function useJobOutput(job: Job | null, active: boolean, onEnded: () => void): OutputData {
  const [buffer, setBuffer] = useState<OutputBuffer | null>(null)
  const [waiting, setWaiting] = useState(false)
  const bufferRef = useRef<OutputBuffer | null>(null)
  const onEndedRef = useRef(onEnded)
  useEffect(() => {
    onEndedRef.current = onEnded
  })
  const jobId = job?.id ?? null
  const jobRunning = job?.state === 'running'
  // 효과는 작업이 바뀌거나 보이게 될 때만 다시 시작한다. 상태가 끝남으로 바뀌어도 같은 반복이 끝까지 받는다.
  const runningRef = useRef(jobRunning)
  useEffect(() => {
    runningRef.current = jobRunning
  })

  useEffect(() => {
    if (!jobId || !active) return
    const ac = new AbortController()
    let timer = 0
    if (bufferRef.current?.jobId !== jobId) bufferRef.current = emptyOutput(jobId)
    let sawRunning = runningRef.current

    const tick = async () => {
      const buf = bufferRef.current ?? emptyOutput(jobId)
      let delay = OUTPUT_INTERVAL_MS
      try {
        const out = await fetchJobOutput(jobId, buf.next, ac.signal)
        if (ac.signal.aborted) return
        const next = appendOutput(buf, out)
        bufferRef.current = next
        setBuffer(next)
        setWaiting(out.waiting === true)
        if (out.state === 'running') sawRunning = true
        if (out.more) delay = 0
        else if (out.state !== 'running') {
          if (sawRunning) onEndedRef.current()
          return
        }
      } catch {
        if (ac.signal.aborted) return
        delay = OUTPUT_INTERVAL_MS * 3
      }
      timer = window.setTimeout(tick, delay)
    }
    void tick()
    return () => {
      ac.abort()
      window.clearTimeout(timer)
    }
  }, [jobId, active])

  return { output: buffer?.jobId === jobId ? buffer : null, waiting: jobRunning && waiting }
}

// useKeepAlive는 running인 동안 세션 활동을 알린다. 창이 보이는 동안만 켠다(부르는 쪽이 active를 함께 넘긴다).
export function useKeepAlive(running: boolean) {
  useEffect(() => {
    if (!running) return
    reportActivity()
    const timer = window.setInterval(reportActivity, ACTIVITY_MS)
    return () => window.clearInterval(timer)
  }, [running])
}
