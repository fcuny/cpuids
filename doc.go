// Package cpuids resolves CPU vendor/family/model IDs (x86) and
// implementer/part IDs (ARM) to human-readable product information.
//
// It is the same idea as pci.ids for PCI devices: an aggregator of existing,
// scattered ID→name mappings, not a new primary source of truth. The data is a
// compilation of vendor-assigned facts; see the repository README and LICENSE
// for the licensing rationale.
//
// The package is pure: the database is embedded at compile time via go:embed,
// so each tagged release pins code and data together and no runtime fetch or
// file I/O is needed. OS-specific convenience helpers live in subpackages such
// as cpuids/linuxcpuinfo.
//
// v1 is exact-lookup only. There is no range/filter query builder;
// GenerationRank is carried in the data for a future one but querying on it is
// out of scope for now.
package cpuids
