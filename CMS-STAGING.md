# CMS staging

The CMS uses public.site_content. GET /api/site-content?language=it&section=Header
returns {"fields":{...}}; PUT /admin/site-content requires an admin bearer JWT
and {"language":"it","section":"Header","fields":{"Header.title":"..."}}.

Railway builds both main and migrate, then runs ./migrate as its pre-deploy command
on the private database network, without SSH. The migration command refuses to run
unless RAILWAY_ENVIRONMENT_NAME is exactly staging. Do not promote this configuration
to production without a separate reviewed migration plan.

The migration uses a transaction and an advisory lock, preserves existing content,
checks the existing table shape and key, and fails on incompatible schemas. It can
be rerun. No DROP, DELETE, or TRUNCATE is performed. The prior stronger migration
mentioned in the handoff was not available; this is a reviewed adaptation of the
attached initial migration.

Validation after deployment: reject unauthenticated PUT with 401; authenticate
through /admin/login; save a temporary Header field; GET from a new request and
compare; restart backend and GET again; restore any modified copy. Check all three
languages and the frontend editor. A successful deploy alone does not prove CMS
persistence. The editor checks a fresh GET before reporting a successful save.
