import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ItemsEnvelope, WebhookEndpointItem } from "./types";
import { mockEnabled, mockWebhookEndpoints } from "./mock";

export interface WebhookEndpointsResult {
  items: WebhookEndpointItem[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// The adapter emits flat values; normalize every row into the strict shape the
// table relies on, dropping nothing and never throwing on a missing field.
function normalize(raw: Record<string, unknown>): WebhookEndpointItem {
  const rawEvents = raw.enabled_events;
  const enabledEvents = Array.isArray(rawEvents)
    ? rawEvents.map((e) => (e ?? "").toString()).filter((e) => e !== "")
    : [];
  return {
    id: (raw.id ?? "").toString(),
    url: (raw.url ?? "").toString(),
    status: (raw.status ?? "").toString(),
    enabledEvents,
    apiVersion: (raw.api_version ?? "").toString()
  };
}

/** True when Stripe is NOT delivering to this endpoint — the readable signal. */
export function isEndpointDisabled(e: WebhookEndpointItem): boolean {
  return e.status.trim().toLowerCase() !== "enabled";
}

/**
 * Every Stripe webhook endpoint the instance configures — the webhook-health
 * pillar, the contract's canonical readable signal. `enabledEvents` is the list
 * of subscribed event types; `status` is "enabled" / "disabled".
 */
export function useWebhookEndpoints(instanceId: string | undefined): WebhookEndpointsResult {
  const mock = mockEnabled();
  // Under `?mock` pass an undefined handle so `useSurfaceQuery` stays disabled
  // (`enabled: !!instanceId`) — the hook is still called for stable order, but
  // it issues zero network and we return the fixture below.
  const query = useSurfaceQuery<ItemsEnvelope<WebhookEndpointItem>>(
    mock ? undefined : instanceId,
    "list-webhook-endpoints",
    {}
  );

  if (mock) {
    return { items: mockWebhookEndpoints(), isLoading: false, isError: false, error: null };
  }

  const raw = (query.data?.items ?? []) as unknown as Array<Record<string, unknown>>;
  return {
    items: raw.map(normalize),
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
