import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OverviewDashboard } from "@/features/dashboard/components/overview/overview-dashboard";
import * as keysApi from "@/features/keys/api";
import { api } from "@/lib/api";
import { useAuthStore } from "@/stores/auth-store";
import { useSystemConfigStore } from "@/stores/system-config-store";

vi.mock(
  "@/features/dashboard/components/overview/performance-health-panel",
  () => ({ PerformanceHealthPanel: () => null }),
);

let client: QueryClient;

function mockKeys(key: string) {
  vi.spyOn(api, "get").mockImplementation(async (url) => {
    switch (url) {
      case "/api/token/?p=1&size=10":
        return {
          data: {
            success: true,
            data: { items: [{ id: 1, name: "App key", key, status: 1 }] },
          },
        };
      case "/api/user/models":
        return { data: { success: true, data: ["gpt-4o-mini"] } };
      case "/api/data/self":
        return { data: { success: true, data: [] } };
      case "/api/status":
        return {
          data: {
            data: {
              api_info_enabled: false,
              announcements_enabled: false,
              faq_enabled: false,
              uptime_kuma_enabled: false,
            },
          },
        };
      default:
        throw new Error(`Unexpected dashboard request: ${url}`);
    }
  });
}

async function renderOverview() {
  const router = createRouter({
    routeTree: createRootRoute({ component: OverviewDashboard }),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  await router.load();
  const view = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  // The request example sits in the setup guide; open it when it is folded.
  const toggle = await screen.findByRole("button", { name: "Setup guide" });
  if (toggle.getAttribute("aria-expanded") === "false") {
    await userEvent.click(toggle);
  }
  return view;
}

beforeEach(() => {
  window.localStorage.clear();
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true);
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: "u",
    role: 10,
    quota: 1000000,
    used_quota: 1000,
    request_count: 1,
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});

afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  useAuthStore.setState(useAuthStore.getInitialState(), true);
});

describe("dashboard request example with hash-only keys", () => {
  it("offers the real copy button for a key that can be read", async () => {
    mockKeys("abcd**********wxyz");
    await renderOverview();
    expect(
      await screen.findByRole("button", { name: "Copy ready-to-run curl" }),
    ).toBeInTheDocument();
  });

  it("shows no copy button and only a masked key for a hash-only key", async () => {
    const fetchKey = vi.spyOn(keysApi, "fetchTokenKey");
    mockKeys("****************");
    await renderOverview();
    expect(
      await screen.findByText("Create a new API key to copy a real request"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Copy ready-to-run curl" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/\*{4}/)).not.toBeInTheDocument();
    expect(fetchKey).not.toHaveBeenCalled();
  });
});
