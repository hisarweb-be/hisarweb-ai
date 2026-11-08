import { t } from "i18next";

// With HW_TOKEN_KEY_HASH_ONLY a key is shown once, when it is created, and the
// server cannot give it back later: its list answer is all asterisks and
// POST /api/token/:id/key answers 409 KEY_NOT_RECOVERABLE. The chat and the
// dashboard therefore never ask for such a key; they point to "create a new key".
// Choice (G2): no new backend contract, the client reads the existing mask.

export const KEY_NOT_RECOVERABLE = "KEY_NOT_RECOVERABLE";

/** The masked key of a list answer; only a hash-only key is asterisks all through. */
export function hwKeyRecoverable(maskedKey?: string): boolean {
  return !/^\*+$/.test(maskedKey ?? "");
}

export function hwKeyNotRecoverableMessage(): string {
  return t(
    "This key was shown once when it was created and cannot be shown again. Create a new key on the API keys page.",
  );
}

/** Throws our own message for the 409 of a hash-only key (it wins over the server text). */
export function hwThrowIfNotRecoverable(result: { success: boolean; code?: string }): void {
  if (result.code === KEY_NOT_RECOVERABLE) {
    throw new Error(hwKeyNotRecoverableMessage());
  }
}
