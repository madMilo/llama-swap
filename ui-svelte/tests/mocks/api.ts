import { createServer } from "node:http";
import { URL } from "node:url";
import type { ServerResponse } from "node:http";
import modelsFixture from "./fixtures/models.json";
import type { APIEventEnvelope, LogData, Metrics, Model, ReqRespCapture, VersionInfo } from "../../src/lib/types";

export interface MockApiServer {
  url: string;
  close: () => Promise<void>;
}

const models = modelsFixture as Model[];

const versionInfo: VersionInfo = {
  build_date: "2025-02-12T12:00:00Z",
  commit: "deadbeef",
  version: "v0.9.0-mock",
};

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

const capturePayload = Buffer.from(
  JSON.stringify({
    model: models[0]?.id ?? "llama3-8b-instruct",
    messages: [{ role: "user", content: "Hello from the mock API" }],
  })
).toString("base64");

const captureResponse = Buffer.from(
  JSON.stringify({
    id: "chatcmpl-mock",
    choices: [{ message: { role: "assistant", content: "Mock response" } }],
  })
).toString("base64");

const captures: ReqRespCapture[] = [
  {
    id: 0,
    req_path: "/v1/chat/completions",
    req_headers: { "content-type": "application/json" },
    req_body: capturePayload,
    resp_headers: { "content-type": "application/json" },
    resp_body: captureResponse,
  },
];

const logEntries: LogData[] = [
  { source: "proxy", data: "[info] Proxy started on :8080\n" },
  { source: "upstream", data: "[info] Upstream connected to mock backend\n" },
];

function jsonResponse(res: ServerResponse, status: number, body: unknown): void {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

function sendEvent(res: ServerResponse, envelope: APIEventEnvelope): void {
  res.write(`data: ${JSON.stringify(envelope)}\n\n`);
}

export async function startMockApiServer(port = 0): Promise<MockApiServer> {
  const sseClients = new Set<ServerResponse>();

  const server = createServer((req, res) => {
    const requestUrl = new URL(req.url ?? "/", "http://localhost");
    const path = requestUrl.pathname;

    if (path === "/api/version" && req.method === "GET") {
      jsonResponse(res, 200, versionInfo);
      return;
    }

    if ((path === "/api/models" || path === "/api/models/") && req.method === "GET") {
      jsonResponse(res, 200, models);
      return;
    }

    if (path.startsWith("/api/models/unload") && req.method === "POST") {
      res.writeHead(204);
      res.end();
      return;
    }

    if (path === "/api/events" && req.method === "GET") {
      res.writeHead(200, {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
      });
      res.write("\n");
      sseClients.add(res);

      const envelope: APIEventEnvelope[] = [
        { type: "modelStatus", data: JSON.stringify(models) },
        { type: "logData", data: JSON.stringify(logEntries[0]) },
        { type: "logData", data: JSON.stringify(logEntries[1]) },
        { type: "metrics", data: JSON.stringify(metrics) },
      ];

      envelope.forEach((payload) => sendEvent(res, payload));

      req.on("close", () => {
        sseClients.delete(res);
      });
      return;
    }

    if (path.startsWith("/api/captures/") && req.method === "GET") {
      const id = Number(path.split("/").pop());
      const capture = captures.find((entry) => entry.id === id);
      if (!capture) {
        res.writeHead(404);
        res.end();
        return;
      }
      jsonResponse(res, 200, capture);
      return;
    }

    if (path.startsWith("/upstream/") && req.method === "GET") {
      res.writeHead(200, { "Content-Type": "text/plain" });
      res.end("Mock upstream response");
      return;
    }

    if (path === "/v1/audio/voices" && req.method === "GET") {
      jsonResponse(res, 200, { voices: ["coral", "alloy", "nova"] });
      return;
    }

    if (path === "/v1/chat/completions" && req.method === "POST") {
      res.writeHead(200, {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
      });
      res.write(
        "data: {\"choices\":[{\"delta\":{\"content\":\"Hello from mock chat\"}}]}\n\n"
      );
      res.write("data: [DONE]\n\n");
      res.end();
      return;
    }

    if (path === "/v1/images/generations" && req.method === "POST") {
      jsonResponse(res, 200, {
        created: Math.floor(Date.now() / 1000),
        data: [{ url: "https://example.com/mock-image.png" }],
      });
      return;
    }

    if (path === "/v1/audio/transcriptions" && req.method === "POST") {
      jsonResponse(res, 200, { text: "Mock transcription" });
      return;
    }

    if (path === "/v1/audio/speech" && req.method === "POST") {
      const audioBuffer = Buffer.from("ID3MockAudio");
      res.writeHead(200, { "Content-Type": "audio/mpeg" });
      res.end(audioBuffer);
      return;
    }

    res.writeHead(404);
    res.end();
  });

  await new Promise<void>((resolve) => {
    server.listen(port, "127.0.0.1", () => resolve());
  });

  const address = server.address();
  const portNumber = typeof address === "object" && address ? address.port : port;
  const url = `http://127.0.0.1:${portNumber}`;

  return {
    url,
    close: async () => {
      for (const client of sseClients) {
        client.end();
      }
      await new Promise<void>((resolve, reject) => {
        server.close((err) => (err ? reject(err) : resolve()));
      });
    },
  };
}
