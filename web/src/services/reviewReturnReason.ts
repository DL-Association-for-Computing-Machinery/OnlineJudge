const encoder = new TextEncoder()

export function validReturnReason(reason: unknown): reason is string {
  return typeof reason === 'string' && reason.length > 0 && encoder.encode(reason).length <= 512 &&
    !/\p{Cc}|\p{Cs}|^\p{White_Space}|\p{White_Space}$/u.test(reason)
}
