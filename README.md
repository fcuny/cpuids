# cpuids

Resolve CPU IDs to human-readable product names — the thing `pci.ids` does for
PCI devices, but for CPUs.

- **x86**: raw CPUID vendor string + effective family/model → product / microarchitecture
- **ARM**: MIDR implementer + part → product / microarchitecture

## What this is, and what it isn't

It **is** an aggregator. The mappings it serves already exist, scattered across
per-tool source files that nobody had normalized into one machine-consumable
artifact:

| Vendor | Upstream source | Notes |
|---|---|---|
| Intel | Linux kernel `arch/x86/include/asm/intel-family.h` | flat `#define INTEL_FAM6_<NAME> <hex>` table |
| ARM | `util-linux` `sys-utils/lscpu-arm.c` | `{ 0xNNN, "Name" }` arrays keyed on implementer + part; cross-checked against `github.com/arm-software/data` |
| AMD | re-derived from AMD PPR documents (`internal/ingest/parse/amd/amd_families.json`) | no clean upstream table exists; kept deliberately conservative |

It **is not** a new primary source of truth, a hosted API, or a
category-defining public standard. It's a quiet personal utility. There is no
external-contribution pipeline for v1.

## Using the Go module

```go
import "fcuny.net/cpuids"

m, ok := cpuids.Resolve(cpuids.X86Key{Vendor: "GenuineIntel", Family: 6, Model: 143})
// m.Vendor == "Intel", m.Microarch == "Sapphire Rapids", m.Segment == "server"

m, ok = cpuids.Resolve(cpuids.ARMKey{ImplementerID: 0x41, PartID: 0xd4f})
// m.Name == "Neoverse-V2", m.Vendor == "ARM Ltd"
```

- `Resolve` has map-style not-found semantics: `(zero Model, false)`.
- `X86Key.Vendor` takes the **raw CPUID vendor string** as-is — exactly what you
  already have from `/proc/cpuinfo` or a raw CPUID call. No caller-side
  translation table; the human-readable vendor name is a field on the returned
  `Model`.
- `Family`/`Model` must be **effective** values (extended bits folded in).
  `/proc/cpuinfo` already reports effective values; code reading raw CPUID
  leaves must fold them — `cpuids.EffectiveFamily` / `cpuids.EffectiveModel` do
  exactly that.
- The database is embedded with `go:embed`, so a tagged release pins code and
  data together — no runtime fetch.

### `cpuids/linuxcpuinfo`

OS-specific convenience, kept out of the core package so the core stays pure:

```go
import "fcuny.net/cpuids/linuxcpuinfo"

m, ok := linuxcpuinfo.ResolveFromCPUInfo() // parses /proc/cpuinfo, then Resolve
```

### v1 scope

Exact lookup only. No range/filter query builder — `generation_rank` is carried
in the data for a future one, but building the query API now would be
speculative. No `stepping`-level granularity. No macOS/Windows helpers.

## Artifacts

Each release attaches:

- `cpu_models.json` — the canonical database (also checked in at
  [`data/cpu_models.json`](data/cpu_models.json) for git-diffable history)
- `cpu_models.sql` / `cpu_models.db` — SQLite build output, generated from the
  JSON, never hand-edited

Schema and the per-row provenance fields are documented in
[`internal/dataset/dataset.go`](internal/dataset/dataset.go).

## Regenerating the data

```
just ingest   # fetch live sources, reparse, rewrite data/ + build/
just sqlite   # rebuild the SQLite artifact from the committed JSON
just ci       # gofmt check + vet + test + build
```

The pipeline:

1. `sources.yaml` pins each source's repo, path, and ref, plus the last-seen
   commit SHA.
2. A daily GitHub Action fetches each file by raw content (no clone) and stops
   if nothing changed.
3. Per-source parsers extract only `(id, name)` fact pairs — upstream comments
   and prose are discarded.
4. `overrides.yaml` — hand-maintained corrections (segment, `generation_rank`,
   aliases, notes, display names) merged on top of parsed data.
5. Validation before publish: schema checks, monotonic `generation_rank` per
   vendor/arch group, and a count-drop **canary** that refuses to publish when a
   parser's output collapses.
6. The bot opens a PR with the diff; a human reviews and merges.
7. Merging a tag cuts a release with the JSON + SQLite attached.

## Licensing

- **Code**: BSD-3-Clause ([`LICENSE`](LICENSE)).
- **Generated data**: CC0 / BSD-3-Clause ([`data/README.md`](data/README.md)).

ID→name mappings are facts — the vendor assigned the ID and named the product —
and copyright protects expression, not facts. `pci.ids` rests on the same
basis. No upstream source file is committed to this repo, not even as a test
fixture: parser tests use synthetic fixtures. Where a source's text is more
than a bare vendor-assigned fact, the value is re-derived from primary vendor
documentation rather than copied. None of the upstream (mostly GPL) licenses
propagate to what's shipped here.
