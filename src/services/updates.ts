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
  // The channel and `allowDowngrade` are part of the key: a check that may
  // offer an older release (channel switch) is a different question.
  check: (allowDowngrade = false) =>
    [...updatesQueryKeys.all, 'check', allowDowngrade] as const,
  pending: () => [...updatesQueryKeys.all, 'pending'] as const,
}

/**
 * The update check. `enabled: false` keeps the app offline (startup check
 * disabled in preferences) until the user asks explicitly.
 *
 * `allowDowngrade: true` additionally offers the channel's newest release when
 * it is older than the running build — that is how a preview build rejoins the
 * stable line (Preferences → General asks this question, the startup banner
 * never does).
 */
export function useUpdateCheck({
  enabled = true,
  allowDowngrade = false,
}: { enabled?: boolean; allowDowngrade?: boolean } = {}) {
  return useQuery({
    queryKey: updatesQueryKeys.check(allowDowngrade),
    queryFn: async () =>
      unwrapResult(await commands.checkForUpdates(allowDowngrade)),
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
    mutationFn: async (allowDowngrade: boolean) =>
      unwrapResult(await commands.downloadUpdate(allowDowngrade)),
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
    mutationFn: async (allowDowngrade: boolean) =>
      unwrapResult(await commands.applyUpdate(allowDowngrade)),
    onSuccess: result => {
      logger.info('Update applied', {
        from: result.fromVersion,
        to: result.toVersion,
        restart: result.restart,
        downgrade: result.downgrade,
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
      void queryClient.invalidateQueries({ queryKey: updatesQueryKeys.all })
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
