import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { hwKeyNotRecoverableMessage } from "@/features/hw/lib/key-recoverable";

import * as keysApi from "../../api";
import { apiKeySchema } from "../../types";
import { ApiKeyCell } from "../api-keys-cells";
import { ApiKeysProvider, useApiKeys } from "../api-keys-provider";

vi.mock("../../api", () => ({
  fetchTokenKey: vi.fn(),
  fetchTokenKeysBatch: vi.fn(),
}));

const row = (key: string) =>
  apiKeySchema.parse({
    id: 7,
    name: "app",
    key,
    status: 1,
    remain_quota: 0,
    used_quota: 0,
    unlimited_quota: true,
    expired_time: -1,
    created_time: 0,
    accessed_time: 0,
    group: "default",
    model_limits_enabled: false,
  });

function Probe() {
  const { resolveRealKey, resolveRealKeysBatch } = useApiKeys();
  return (
    <>
      <button onClick={() => resolveRealKey(7)}>one</button>
      <button
        onClick={async () =>
          document.body.setAttribute(
            "data-batch",
            JSON.stringify(await resolveRealKeysBatch([7])),
          )
        }
      >
        batch
      </button>
    </>
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
  document.body.removeAttribute("data-batch");
});

describe("keys page with hash-only keys", () => {
  it("a readable key keeps an enabled copy button", () => {
    render(
      <ApiKeysProvider>
        <ApiKeyCell apiKey={row("abcd**********wxyz")} />
      </ApiKeysProvider>,
    );
    const buttons = screen.getAllByRole("button");
    expect(buttons[buttons.length - 1]).toBeEnabled();
  });

  it("a hash-only key has a disabled copy button and never asks the server", async () => {
    render(
      <ApiKeysProvider>
        <ApiKeyCell apiKey={row("****************")} />
      </ApiKeysProvider>,
    );
    const buttons = screen.getAllByRole("button");
    expect(buttons[buttons.length - 1]).toBeDisabled();
    await userEvent.click(buttons[0]); // opens the popover
    expect(keysApi.fetchTokenKey).not.toHaveBeenCalled();
  });

  it("turns a 409 KEY_NOT_RECOVERABLE into our own message", async () => {
    const err = vi.spyOn(toast, "error").mockImplementation(() => 1);
    vi.mocked(keysApi.fetchTokenKey).mockResolvedValue({
      success: false,
      code: "KEY_NOT_RECOVERABLE",
      message: "server text",
    } as never);
    render(
      <ApiKeysProvider>
        <Probe />
      </ApiKeysProvider>,
    );
    await userEvent.click(screen.getByText("one"));
    await waitFor(() =>
      expect(err).toHaveBeenCalledWith(hwKeyNotRecoverableMessage()),
    );
  });

  it("never turns an empty batch answer into a fake sk- key", async () => {
    vi.mocked(keysApi.fetchTokenKeysBatch).mockResolvedValue({
      success: true,
      data: { keys: { 7: "" } },
    });
    render(
      <ApiKeysProvider>
        <Probe />
      </ApiKeysProvider>,
    );
    await userEvent.click(screen.getByText("batch"));
    await waitFor(() =>
      expect(document.body.getAttribute("data-batch")).toBe("{}"),
    );
  });
});
