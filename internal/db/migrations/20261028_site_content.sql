-- Run with cmd/migrate inside a transaction. Existing content is never deleted.
SELECT pg_advisory_xact_lock(20261028, 1);
SET LOCAL lock_timeout = '10s';
CREATE TABLE IF NOT EXISTS public.site_content (
    language TEXT NOT NULL CHECK (language IN ('it', 'en', 'es')),
    section TEXT NOT NULL,
    field_key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (language, section, field_key)
);
DO $$
DECLARE actual TEXT[];
BEGIN
    IF (SELECT relkind FROM pg_class WHERE oid = 'public.site_content'::regclass) <> 'r' THEN
        RAISE EXCEPTION 'site_content must be an ordinary table';
    END IF;
    SELECT array_agg(column_name::text ORDER BY ordinal_position) INTO actual
    FROM information_schema.columns WHERE table_schema='public' AND table_name='site_content';
    IF actual IS DISTINCT FROM ARRAY['language','section','field_key','value','updated_at'] THEN
        RAISE EXCEPTION 'Incompatible site_content columns: %', actual;
    END IF;
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema='public' AND table_name='site_content' AND (
            is_nullable <> 'NO' OR
            (column_name='language' AND data_type <> 'text') OR
            (column_name='section' AND data_type <> 'text') OR
            (column_name='field_key' AND data_type <> 'text') OR
            (column_name='value' AND data_type <> 'text') OR
            (column_name='updated_at' AND (data_type <> 'timestamp with time zone' OR column_default IS NULL))
        )
    ) THEN RAISE EXCEPTION 'Incompatible site_content column types, nullability or timestamp default'; END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        WHERE c.conrelid='public.site_content'::regclass AND c.contype='p'
        AND (SELECT array_agg(a.attname::text ORDER BY k.ord)
             FROM unnest(c.conkey) WITH ORDINALITY AS k(num,ord)
             JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=k.num)
            = ARRAY['language','section','field_key']
    ) THEN RAISE EXCEPTION 'Incompatible site_content primary key'; END IF;
END $$;
-- These named constraints also validate existing rows. Re-running is harmless.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.site_content'::regclass AND conname='site_content_cms_scope_v1') THEN
        ALTER TABLE public.site_content ADD CONSTRAINT site_content_cms_scope_v1 CHECK (
            language IN ('it','en','es') AND section IN ('Header','HeroSection','AboutSection','PricingSection','ContactSection','Footer','SEO')
            AND length(field_key) BETWEEN 1 AND 200 AND length(value)<=20000
        );
    END IF;
END $$;
