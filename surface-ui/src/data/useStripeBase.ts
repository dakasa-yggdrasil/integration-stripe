import { mockEnabled, MOCK_STRIPE_BASE } from "./mock";

/**
 * The native Stripe Dashboard host root used to build "↗" deep-links.
 *
 * HONEST GAP: the instance's dashboard host is NOT returned by any surface query
 * today (the adapter shapes only row fields), so in the live path this resolves
 * to "" and every deep-link degrades to a disabled, explained "↗" (see
 * {@link DeepLinkArrow}). Under `?mock` we return the real dashboard host so the
 * affordance can be reviewed as a working link. When a future surface read
 * exposes the host, wire it in here — every page already routes its links
 * through this one hook.
 */
export function useStripeBase(): string {
  if (mockEnabled()) return MOCK_STRIPE_BASE;
  return "";
}
