// Package linuxcpuinfo resolves the CPU of the current Linux host by reading
// /proc/cpuinfo and calling into the core cpuids package.
//
// It is a thin, OS-specific convenience kept separate from cpuids so that the
// core package stays pure and portable. Future helpers for other platforms
// (macOS sysctl, Windows WMI) would be sibling subpackages.
//
// Note on the extended model arithmetic: the Linux kernel folds the
// extended-family and extended-model CPUID bits into the values it exposes as
// "cpu family" and "model" in /proc/cpuinfo, so this package uses them
// directly. Code that bypasses procfs and reads raw CPUID leaves must do that
// folding itself — see cpuids.EffectiveFamily and cpuids.EffectiveModel.
package linuxcpuinfo

import (
	"bufio"
	"bytes"
	"os"
	"strconv"
	"strings"

	"fcuny.net/cpuids"
)

// CPUInfoPath is the file read by ResolveFromCPUInfo. It is a variable so tests
// can point it at a fixture.
var CPUInfoPath = "/proc/cpuinfo"

// ResolveFromCPUInfo reads CPUInfoPath, builds a key from the first processor
// block, and resolves it. It has map-style not-found semantics: the zero Model
// and false when the file cannot be read, cannot be understood, or the CPU is
// not in the database.
func ResolveFromCPUInfo() (cpuids.Model, bool) {
	b, err := os.ReadFile(CPUInfoPath)
	if err != nil {
		return cpuids.Model{}, false
	}
	return resolve(b)
}

func resolve(b []byte) (cpuids.Model, bool) {
	fields := firstBlock(b)
	if len(fields) == 0 {
		return cpuids.Model{}, false
	}

	// ARM: /proc/cpuinfo carries "CPU implementer" / "CPU part" as hex.
	if impl, ok := fields["cpu implementer"]; ok {
		part, ok2 := fields["cpu part"]
		if !ok2 {
			return cpuids.Model{}, false
		}
		implN, err1 := parseUint(impl)
		partN, err2 := parseUint(part)
		if err1 != nil || err2 != nil {
			return cpuids.Model{}, false
		}
		return cpuids.Resolve(cpuids.ARMKey{
			ImplementerID: uint8(implN),
			PartID:        uint16(partN),
		})
	}

	// x86: "vendor_id", "cpu family", "model". These are already effective
	// values; no extended-bit folding needed here.
	vendor := fields["vendor_id"]
	famStr, ok1 := fields["cpu family"]
	modStr, ok2 := fields["model"]
	if vendor == "" || !ok1 || !ok2 {
		return cpuids.Model{}, false
	}
	fam, err1 := strconv.Atoi(strings.TrimSpace(famStr))
	mod, err2 := strconv.Atoi(strings.TrimSpace(modStr))
	if err1 != nil || err2 != nil {
		return cpuids.Model{}, false
	}
	return cpuids.Resolve(cpuids.X86Key{Vendor: vendor, Family: fam, Model: mod})
}

// firstBlock returns the key/value pairs of the first processor block, with
// lowercased keys and trimmed values. Blocks are separated by a blank line.
func firstBlock(b []byte) map[string]string {
	out := make(map[string]string)
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			if len(out) > 0 {
				break
			}
			continue
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}

// parseUint parses a decimal or 0x-prefixed value.
func parseUint(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseUint(s[2:], 16, 64)
	}
	return strconv.ParseUint(s, 10, 64)
}
