<script lang="ts">
  import { models } from "../stores/api";
  import type { Model } from "../lib/types";

  function formatMB(value?: number): string {
    if (!value) {
      return "—";
    }
    return `${value.toLocaleString()} MB`;
  }

  function formatDelta(initial?: number, measured?: number): string {
    if (!initial || !measured) {
      return "—";
    }
    const delta = measured - initial;
    const sign = delta > 0 ? "+" : "";
    return `${sign}${delta.toLocaleString()} MB`;
  }

  function hasMismatch(initial?: number, measured?: number): boolean {
    return Boolean(initial && measured && initial !== measured);
  }

  function getRecommendation(model: Model): { label: string; note?: string } {
    const policy = (model.fitPolicy ?? "").toLowerCase();
    if (policy === "spill") {
      return { label: "spill (configured)" };
    }
    const baseLabel = "evict_to_fit (no --fit)";
    const measuredCpu = model.measuredCpuMB ?? 0;
    const initialCpu = model.initialCpuMB ?? 0;
    if (measuredCpu > 0 && measuredCpu > initialCpu) {
      return { label: baseLabel, note: "Spill recommended: observed host RAM usage exceeds hint." };
    }
    return { label: baseLabel };
  }

  const measuredModels = $derived(
    $models.filter((model) => (model.measuredVramMB ?? 0) > 0 || (model.measuredCpuMB ?? 0) > 0),
  );
</script>

<section class="space-y-4">
  <div class="card">
    <h2 class="text-2xl font-semibold pb-2">Recommendations</h2>
    <p class="text-txtsecondary">
      Models listed here have measured memory footprints. Use discrepancies between initial hints and actuals to refine
      fit policies and memory caps.
    </p>
  </div>

  <div class="card">
    <h3 class="text-xl font-semibold pb-2">Measured footprints</h3>
    {#if measuredModels.length === 0}
      <p class="text-txtsecondary">No measured footprints yet. Start a model to collect measurements.</p>
    {:else}
      <div class="overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead class="text-left text-txtsecondary border-b border-card-border-inner">
            <tr>
              <th>Model</th>
              <th>Initial VRAM</th>
              <th>Measured VRAM</th>
              <th>Δ VRAM</th>
              <th>Initial CPU</th>
              <th>Measured CPU</th>
              <th>Δ CPU</th>
              <th>Fit policy</th>
              <th>Recommendation</th>
            </tr>
          </thead>
          <tbody>
            {#each measuredModels as model}
              {@const recommendation = getRecommendation(model)}
              <tr class="border-b border-card-border-inner">
                <td class="font-medium">{model.name || model.id}</td>
                <td>{formatMB(model.initialVramMB)}</td>
                <td>{formatMB(model.measuredVramMB)}</td>
                <td class:text-warning={hasMismatch(model.initialVramMB, model.measuredVramMB)}>
                  {formatDelta(model.initialVramMB, model.measuredVramMB)}
                </td>
                <td>{formatMB(model.initialCpuMB)}</td>
                <td>{formatMB(model.measuredCpuMB)}</td>
                <td class:text-warning={hasMismatch(model.initialCpuMB, model.measuredCpuMB)}>
                  {formatDelta(model.initialCpuMB, model.measuredCpuMB)}
                </td>
                <td>{model.fitPolicy || "—"}</td>
                <td>
                  <div class="font-medium">{recommendation.label}</div>
                  {#if recommendation.note}
                    <div class="text-txtsecondary text-xs">{recommendation.note}</div>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</section>
