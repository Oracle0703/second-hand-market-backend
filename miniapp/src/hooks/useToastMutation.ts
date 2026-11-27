import Taro from '@tarojs/taro'
import { useCallback, useRef } from 'react'
import { type MutationOptions, type MutationResult, useMutation } from '@/libs/react-query'

const TECHNICAL_ERROR_PREFIXES = ['request failed', 'API response malformed']

export function getMutationErrorMessage(error: unknown, fallback: string): string {
  const message = error instanceof Error ? error.message.trim() : ''
  if (!message || TECHNICAL_ERROR_PREFIXES.some((prefix) => message.startsWith(prefix))) {
    return fallback
  }
  return message
}

type ToastMutationOptions<TData, TVariables> = MutationOptions<TData, TVariables> & {
  errorMessage: string
}

export function useToastMutation<TData = unknown, TVariables = void>(
  options: ToastMutationOptions<TData, TVariables>
): MutationResult<TData, TVariables> {
  const { errorMessage, onError, ...rest } = options
  const inFlightRef = useRef(false)
  const mutation = useMutation<TData, TVariables>({
    ...rest,
    onError: async (error, variables) => {
      void Taro.showToast({ title: getMutationErrorMessage(error, errorMessage), icon: 'none' })
      await onError?.(error, variables)
    }
  })
  const { mutateAsync } = mutation

  const mutate = useCallback(
    (variables: TVariables) => {
      if (inFlightRef.current) {
        return
      }
      inFlightRef.current = true
      mutateAsync(variables)
        .catch(() => undefined)
        .finally(() => {
          inFlightRef.current = false
        })
    },
    [mutateAsync]
  )

  return { ...mutation, mutate }
}
