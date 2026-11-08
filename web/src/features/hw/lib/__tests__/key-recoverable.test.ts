import { beforeEach, describe, expect, it, vi } from "vitest";

import { fetchActiveChatKey } from "@/features/chat/hooks/use-active-chat-key";
import * as keysApi from "@/features/keys/api";

import {
  hwKeyNotRecoverableMessage,
  hwKeyRecoverable,
} from "../key-recoverable";

vi.mock("@/features/keys/api", () => ({
  getApiKeys: vi.fn(),
  fetchTokenKey: vi.fn(),
}));

const list = (items: Array<{ id: number; key: string; status: number }>) =>
  vi.mocked(keysApi.getApiKeys).mockResolvedValue({
    success: true,
    data: { items },
  } as never);

beforeEach(() => vi.resetAllMocks());

describe("hwKeyRecoverable", () => {
  it("sees only an all-asterisk mask as a key that is gone", () => {
    expect(hwKeyRecoverable("****************")).toBe(false);
    expect(hwKeyRecoverable("sk-ab**********wxyz")).toBe(true);
    expect(hwKeyRecoverable("ab****yz")).toBe(true);
    expect(hwKeyRecoverable("")).toBe(true); // empty mask: keep today's behaviour
    expect(hwKeyRecoverable(undefined)).toBe(true);
  });
});

describe("fetchActiveChatKey", () => {
  it("without the flag asks the key route as before", async () => {
    list([{ id: 7, key: "abcd**********wxyz", status: 1 }]);
    vi.mocked(keysApi.fetchTokenKey).mockResolvedValue({
      success: true,
      data: { key: "real" },
    });
    await expect(fetchActiveChatKey()).resolves.toBe("sk-real");
    expect(keysApi.fetchTokenKey).toHaveBeenCalledWith(7);
  });

  it("skips a hash-only key for a key that can still be read", async () => {
    list([
      { id: 1, key: "****************", status: 1 },
      { id: 2, key: "abcd**********wxyz", status: 1 },
    ]);
    vi.mocked(keysApi.fetchTokenKey).mockResolvedValue({
      success: true,
      data: { key: "old" },
    });
    await fetchActiveChatKey();
    expect(keysApi.fetchTokenKey).toHaveBeenCalledWith(2);
  });

  it("with only hash-only keys says to create a new key, without a password prompt", async () => {
    list([{ id: 1, key: "****************", status: 1 }]);
    await expect(fetchActiveChatKey()).rejects.toThrow(
      hwKeyNotRecoverableMessage(),
    );
    expect(keysApi.fetchTokenKey).not.toHaveBeenCalled();
  });

  it("turns a 409 KEY_NOT_RECOVERABLE into the same message", async () => {
    list([{ id: 1, key: "abcd**********wxyz", status: 1 }]);
    vi.mocked(keysApi.fetchTokenKey).mockResolvedValue({
      success: false,
      code: "KEY_NOT_RECOVERABLE",
      message: "server text",
    } as never);
    await expect(fetchActiveChatKey()).rejects.toThrow(
      hwKeyNotRecoverableMessage(),
    );
  });
});
