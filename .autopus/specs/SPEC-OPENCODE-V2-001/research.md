# Primary references

Read on 2026-09-20, installed OpenCode 2.0.10.

- https://opencode.ai/v2/docs/migrate-v1/
- https://opencode.ai/v2/docs/build/plugins
- https://opencode.ai/v2/docs/plugins/
- https://opencode.ai/v2/docs/tools/

V1 function plugins and tool.execute.before/after are replaced by Plugin.define
and tool-domain hook registration. The new plugin API is a real migration;
legacy file-based agents and skills need not be discarded. Native V2 plugins
configuration takes precedence over its legacy alias. Registration is distinct
from successful loading and actual hook execution.

Native generator completeness regression: a fixture with a manually installed
SDK passed, but the same generated import failed without that dependency and
produced only the shell marker. The generator now emits the native object
directly; Plugin.define in the published 2.0.10 package returns its input. The
required host fixture must run without manual SDK installation.
