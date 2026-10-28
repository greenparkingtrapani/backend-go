-- Additive, repeatable migration; no existing reservation records are altered.
CREATE TABLE IF NOT EXISTS public.luggage_requests (
  code TEXT PRIMARY KEY,
  customer_name TEXT NOT NULL,
  email TEXT NOT NULL,
  phone TEXT NOT NULL,
  origin TEXT NOT NULL DEFAULT '',
  dropoff_at TIMESTAMPTZ NOT NULL,
  pickup_at TIMESTAMPTZ NOT NULL,
  bags JSONB NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','received','collected','cancelled','deleted')),
  quoted_cents INTEGER NOT NULL CHECK (quoted_cents >= 0),
  quoted_hours INTEGER NOT NULL CHECK (quoted_hours > 0),
  paid_cents INTEGER NOT NULL DEFAULT 0 CHECK (paid_cents >= 0),
  payment_method TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  collected_at TIMESTAMPTZ,
  CHECK (pickup_at > dropoff_at)
);
CREATE INDEX IF NOT EXISTS luggage_requests_dropoff_idx ON public.luggage_requests (dropoff_at DESC);
