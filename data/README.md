# Generated data

`cpu_models.json` in this directory is the canonical database. It is produced by
the ingestion pipeline (`cmd/cpuids-ingest`) and checked in so its history is
git-diffable. The SQLite artifact attached to releases is a build output
derived from this file and is never hand-edited.

## License of the data

The generated data in this directory is released under
[CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) (public domain
dedication). You may also treat it as BSD-3-Clause if a permissive-license
notice is easier for you to carry.

ID→name mappings are facts: the vendor assigned the numeric ID and named the
product. Copyright protects expression, not facts. `pci.ids` rests on the same
basis. The parsers in this repository extract only the `(id, name)` fact pairs
from their sources and discard upstream comments, prose, and any editorializing
text; where a source's text is more than a bare vendor-assigned fact, the value
is re-derived from primary vendor documentation instead of copied. No upstream
source file is committed here, not even as a test fixture.

The upstream sources carry their own (mostly GPL) licenses; none of that
propagates to this compilation. See `../sources.yaml` for the per-source
detail and `../README.md` for the full rationale.
