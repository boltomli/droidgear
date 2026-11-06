/**
 * Self-update: TanStack Query hooks over the Go updater service
 * (internal/service/updater, exposed by internal/app/update.go).
 *
 * The flow is check → download (staged + SHA-256 verified) → apply (swap the
 * executable + restart). Both the startup banner and Preferences → General use
 * these hooks, and they share one query key so the app talks to GitHub once.
 */
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import i18n from '@/i18n/config'
import { commands, unwrapResult } from '@/lib/app-bindings'
import type { UpdateProgress } from '@/lib/bindings'
import { listen } from '@/lib/shell/events'
import { logger } from '@/lib/logger'

export const updatesQueryKeys = {
  all: ['updates'] as const,
  check: () => [...updatesQueryKeys.all, 'check'] as const,
  pending: () => [...updatesQueryKeys.all, 'pending'] as const,
}

/**
 * The update check. `enabled: false` keeps the app offline (startup check
 * disabled in preferences) until the user asks explicitly.
 */
export function useUpdateCheck({ enabled = true }: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: updatesQueryKeys.check(),
    queryFn: async () => unwrapResult(await commands.checkForUpdates()),
    enabled,
    // A release does not appear mid-session often enough to poll.
    staleTime: 1000 * 60 * 15,
    retry: false,
  })
}

/** The update downloaded on this machine, waiting to be applied. */
export function usePendingUpdate() {
  return useQuery({
    queryKey: updatesQueryKeys.pending(),
    queryFn: async () => unwrapResult(await commands.getPendingUpdate()),
    staleTime: 0,
  })
}

export function useDownloadUpdate() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async () => unwrapResult(await commands.downloadUpdate()),
    onSuccess: result => {
      logger.info('Update downloaded and verified', {
        version: result.version,
        size: result.size,
      })
      void queryClient.invalidateQueries({
        queryKey: updatesQueryKeys.pending(),
      })
      toast.success(
        i18n.t('updates.downloadComplete', { version: result.version })
      )
    },
    onError: error => {
      logger.error('Update download failed', { error })
      toast.error(i18n.t('updates.downloadFailed'), {
        description: String(error),
      })
    },
  })
}

export function useApplyUpdate() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async () => unwrapResult(await commands.applyUpdate()),
    onSuccess: result => {
      logger.info('Update applied', {
        from: result.fromVersion,
        to: result.toVersion,
        restart: result.restart,
      })
      void queryClient.invalidateQueries({
        queryKey: updatesQueryKeys.pending(),
      })
      if (!result.restart) {
        // The swap succeeded but the automatic restart did not: the app is
        // still the old process, so tell the user to close and reopen it.
        toast.info(
          i18n.t('updates.restartManually', { version: result.toVersion })
        )
      }
    },
    onError: error => {
      logger.error('Update apply failed', { error })
      toast.error(i18n.t('updates.applyFailed'), {
        description: String(error),
      })
    },
  })
}

export function useDismissUpdate() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (version: string) =>
      unwrapResult(await commands.dismissUpdate(version)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: updatesQueryKeys.check() })
    },
  })
}

/**
 * Live download progress from the Go side (`update:progress` events). Returns
 * null until the first event of a download arrives.
 */
export function useUpdateProgress() {
  const [progress, setProgress] = useState<UpdateProgress | null>(null)

  useEffect(() => {
    let unlisten: (() => void) | undefined
    let disposed = false

    void listen<UpdateProgress>('update:progress', event => {
      setProgress(event.payload)
    }).then(stop => {
      if (disposed) stop()
      else unlisten = stop
    })

    return () => {
      disposed = true
      unlisten?.()
    }
  }, [])

  return { progress, reset: () => setProgress(null) }
}
