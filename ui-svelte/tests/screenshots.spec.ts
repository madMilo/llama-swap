import { test, type Page } from "@playwright/test";
import type { APIEventEnvelope, LogData, Metrics, Model } from "../src/lib/types";
import modelsFixture from "./mocks/fixtures/models.json";
import { startMockApiServer, type MockApiServer } from "./mocks/api";

let mockServer: MockApiServer;

function buildSsePayload(): string {
  const models = modelsFixture as Model[];
  const logEntries: LogData[] = [
    { source: "proxy", data: "[info] Proxy started on :8080\n" },
    { source: "upstream", data: "[info] Upstream connected to mock backend\n" },
  ];

  const metrics: Metrics[] = [
    {
      id: 0,
      timestamp: new Date().toISOString(),
      model: models[0]?.id ?? "llama3-8b-instruct",
      cache_tokens: 128,
      input_tokens: 512,
      output_tokens: 256,
      prompt_per_second: 120.5,
      tokens_per_second: 98.3,
      duration_ms: 2200,
      has_capture: true,
    },
  ];

  const envelopes: APIEventEnvelope[] = [
    { type: "modelStatus", data: JSON.stringify(models) },
    { type: "logData", data: JSON.stringify(logEntries[0]) },
    { type: "logData", data: JSON.stringify(logEntries[1]) },
    { type: "metrics", data: JSON.stringify(metrics) },
  ];

  return envelopes.map((payload) => `data: ${JSON.stringify(payload)}\n\n`).join("");
}

async function routeMockApi(page: Page, apiUrl: string) {
  const ssePayload = buildSsePayload();

  await page.route(["**/api/**", "**/upstream/**", "**/v1/**"], async (route) => {
    const requestUrl = new URL(route.request().url());

    if (requestUrl.pathname === "/api/events") {
      await route.fulfill({
        status: 200,
        headers: {
          "content-type": "text/event-stream",
          "cache-control": "no-cache",
          connection: "keep-alive",
        },
        body: ssePayload,
      });
      return;
    }

    const proxyUrl = new URL(requestUrl.pathname + requestUrl.search, apiUrl);
    const response = await route.fetch({ url: proxyUrl.toString() });
    await route.fulfill({ response });
  });
}

test.beforeAll(async () => {
  mockServer = await startMockApiServer();
});

test.afterAll(async () => {
  await mockServer.close();
});

test("capture route screenshots", async ({ page }, testInfo) => {
  await routeMockApi(page, mockServer.url);

  const routes = [
    { path: "/models", label: "models" },
    { path: "/running", label: "running" },
    { path: "/activity", label: "activity" },
    { path: "/logviewer", label: "logviewer" },
    { path: "/logs", label: "logs" },
    { path: "/playground", label: "playground" },
  ];

  for (const route of routes) {
    await page.goto(route.path);
    await page.getByRole("link", { name: "Playground" }).waitFor();

    if (route.path === "/models") {
      await page.getByText("Llama 3 8B Instruct").waitFor();
    }

    if (route.path === "/activity") {
      await page.getByRole("heading", { name: "Activity" }).waitFor();
      await page.getByRole("button", { name: "View" }).waitFor();
    }

    if (route.path === "/logs") {
      await page.getByText("Proxy Logs").first().waitFor();
    }

    await page.waitForTimeout(500);
    await page.screenshot({
      path: testInfo.outputPath(`${route.label}.png`),
      fullPage: true,
    });
  }
});
